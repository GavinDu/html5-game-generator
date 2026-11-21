// Package playteststage provides an isolated loopback game/package server for
// pre-publication browser validation. It is NOT a production HTTP endpoint.
// The public store is never marked GameReady; all shadow state and assets stay
// inside a per-run in-memory snapshot reachable only on 127.0.0.1.
package playteststage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"doodlequest/internal/api"
	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Server owns an ephemeral, local-only browser-validation transport.
// Close it immediately after both browser workers finish or fail.
type Server struct {
	BaseURL    string
	httpServer *http.Server
	listener   net.Listener
}

func (s *Server) Close() error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

// noStoreWriter prevents a private staged GameReady shadow from producing
// cacheable public response headers. The public copy is never modified.
type noStoreWriter struct{ http.ResponseWriter }

func (w noStoreWriter) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.ResponseWriter.WriteHeader(code)
}
func (w noStoreWriter) Write(data []byte) (int, error) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	return w.ResponseWriter.Write(data)
}

// Open creates a local-only view of exactly one unready generated game.
// The staging copy's Ready status is a *private transport implementation*
// detail; production store status, visibility, moderation and manifest never
// change. The generated IR must be complete and every asset byte validated
// before the listener opens. This function does not approve the game.
func Open(source store.Store, gameID string, generated *gameir.Document) (*Server, error) {
	if source == nil || strings.TrimSpace(gameID) == "" || generated == nil {
		return nil, errors.New("playtest staging: missing source store, game id or canonical IR")
	}
	original, ok := source.GetGame(gameID)
	if !ok || original == nil {
		return nil, fmt.Errorf("playtest staging: game %q not found", gameID)
	}
	if original.Status != model.GameGenerating {
		return nil, fmt.Errorf("playtest staging: game %q must be generating, not %q", gameID, original.Status)
	}
	if generated.Game.ID != gameID {
		return nil, fmt.Errorf("playtest staging: generated IR identity %q differs from game %q", generated.Game.ID, gameID)
	}
	if err := gameir.Validate(generated); err != nil {
		return nil, fmt.Errorf("playtest staging: invalid canonical IR: %w", err)
	}

	// Validate everything before starting any network listener.
	type artifact struct {
		uri  string
		data []byte
		mime string
	}
	artifacts := make([]artifact, 0, len(generated.Assets))
	for _, asset := range generated.Assets {
		payload, mime, found := source.GetArtifact(gameID, asset.URI)
		if !found || len(payload) == 0 {
			return nil, fmt.Errorf("playtest staging: missing asset %q", asset.ID)
		}
		if asset.MIMEType != "" && asset.MIMEType != mime {
			return nil, fmt.Errorf("playtest staging: wrong MIME type for asset %q", asset.ID)
		}
		digest := sha256.Sum256(payload)
		if asset.SHA256 != "" && !strings.EqualFold(asset.SHA256, hex.EncodeToString(digest[:])) {
			return nil, fmt.Errorf("playtest staging: asset %q SHA-256 mismatch", asset.ID)
		}
		artifacts = append(artifacts, artifact{uri: asset.URI, data: payload, mime: mime})
	}

	isolated := store.NewMemoryStore()
	shadow := *original
	shadow.Status = model.GameReady
	shadow.Visibility = model.VisibilityPublic
	shadow.ModerationStatus = "approved"
	shadow.Quarantined = false
	shadow.GameIR = generated
	shadow.Manifest = &model.BundleManifest{
		GameID: gameID, Title: generated.Game.Title, Version: 1,
		Shim: "gameir/v1.2", Entry: "game-ir.json",
	}
	isolated.CreateGame(&shadow)
	for _, asset := range artifacts {
		isolated.PutArtifact(gameID, asset.uri, asset.data, asset.mime)
	}

	// Reuse the exact production renderer and package serializer against
	// the private snapshot, but expose ONLY the read-only resources required
	// by the browser worker. No feeds, ingest, social, or mutation endpoints.
	routes := api.New(isolated, pipeline.NewRunner(isolated)).Routes()
	mux := http.NewServeMux()
	for _, pattern := range []string{
		"GET /player",
		"GET /web/pixi-runtime.js",
		"GET /web/vendor/pixi.min.mjs",
		"GET /web/vendor/rapier.es.js",
		"GET /v1/games/{id}",
		"GET /v1/games/{id}/package",
		"GET /v1/games/{id}/assets/{asset}",
	} {
		mux.Handle(pattern, routes)
	}
	guarded := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mux.ServeHTTP(noStoreWriter{ResponseWriter: w}, req)
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("playtest staging: bind loopback: %w", err)
	}
	server := &http.Server{
		Handler:           guarded,
		ReadHeaderTimeout: 5 * time.Second,
	}
	stage := &Server{BaseURL: "http://" + listener.Addr().String(), httpServer: server, listener: listener}
	go func() { _ = server.Serve(listener) }()
	return stage, nil
}
