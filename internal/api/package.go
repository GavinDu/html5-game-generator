package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

const PackageVersion = "2.0"

type GamePackageAsset struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	URL      string `json:"url"`
	MIMEType string `json:"mimeType,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Source   string `json:"source,omitempty"`
	AltText  string `json:"altText,omitempty"`
}

// GamePackage is the canonical, engine-independent delivery envelope.
// The IR keeps package-relative asset URIs; URLs here are delivery locations.
type GamePackage struct {
	PackageVersion string             `json:"packageVersion"`
	GameID         string             `json:"gameId"`
	IR             *gameir.Document   `json:"ir"`
	Assets         []GamePackageAsset `json:"assets"`
	// IRHash identifies this exact IR content; leaderboard boards are
	// keyed by it, so any IR change starts a fresh board (Phase C).
	IRHash string `json:"irHash"`
}

// canDeliver reports whether viewer may load and play g through the
// delivery endpoints (package, assets, game-ir, game record). The feed
// has always applied the author's sharing rules and the moderation
// state; the delivery endpoints applied none of them, so a private,
// pending, rejected or quarantined game stayed playable by anyone who
// knew its ID (architecture review 2026-10-23). The author always
// keeps access to their own game (creator preview and repair routes).
// Anyone else needs a ready, unquarantined, moderation-cleared game
// shared with them: public, or a groups game whose groups include
// them. The /player shell itself carries no game data and stays
// open; it renders the runtime's error state when delivery refuses.
func (s *Server) canDeliver(g *model.Game, viewer string) bool {
	if viewer != "" && viewer == g.OwnerID {
		return true
	}
	if g.Status != model.GameReady || g.Quarantined {
		return false
	}
	if g.ModerationStatus == "pending" || g.ModerationStatus == "rejected" {
		return false
	}
	switch g.Visibility {
	case model.VisibilityPublic:
		return true
	case model.VisibilityGroups:
		for _, gid := range g.GroupIDs {
			gr, ok := s.store.GetGroup(gid)
			if !ok {
				continue
			}
			if gr.OwnerID == viewer {
				return true
			}
			for _, m := range gr.MemberIDs {
				if m == viewer {
					return true
				}
			}
		}
	}
	return false
}

// deliverableGame loads the {id} game and enforces canDeliver,
// answering 404 for anything the viewer may not see — restricted
// games stay indistinguishable from missing ones.
func (s *Server) deliverableGame(w http.ResponseWriter, r *http.Request) (*model.Game, bool) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok || !s.canDeliver(game, userID(r)) {
		writeErr(w, http.StatusNotFound, "game not found")
		return nil, false
	}
	return game, true
}

// packageIsPublicImmutable limits shared caching to ready, public content whose
// delivery eligibility does not depend on the requesting user's identity.
func packageIsPublicImmutable(game *model.Game) bool {
	return game != nil &&
		game.Status == model.GameReady &&
		game.Visibility == model.VisibilityPublic &&
		!game.Quarantined &&
		game.ModerationStatus != "pending" &&
		game.ModerationStatus != "rejected"
}

// setPackageCacheHeaders gives immutable public artifacts a strong ETag and
// bounded shared caching. Owner previews and restricted content are never
// cacheable by shared proxies. It returns true when a public conditional GET
// can be answered with 304 Not Modified.
func setPackageCacheHeaders(w http.ResponseWriter, r *http.Request, game *model.Game, etag string) bool {
	w.Header().Set("ETag", etag)
	if !packageIsPublicImmutable(game) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Vary", "X-User-ID")
		return false
	}
	w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
	w.Header().Set("Vary", "Accept-Encoding")
	return r.Header.Get("If-None-Match") == etag || r.Header.Get("If-None-Match") == "*"
}

func strongETag(data []byte) string {
	sum := sha256.Sum256(data)
	return "\"" + hex.EncodeToString(sum[:]) + "\""
}

func (s *Server) handleGetPackage(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "Game IR has not been published")
		return
	}
	pkg := GamePackage{
		PackageVersion: PackageVersion,
		GameID:         game.ID,
		IR:             game.GameIR,
		Assets:         make([]GamePackageAsset, 0, len(game.GameIR.Assets)),
		IRHash:         irHashOf(game.GameIR),
	}
	for _, asset := range game.GameIR.Assets {
		pkg.Assets = append(pkg.Assets, GamePackageAsset{
			ID: asset.ID, Kind: asset.Kind, URI: asset.URI,
			URL:      "/v1/games/" + game.ID + "/assets/" + url.PathEscape(asset.ID),
			MIMEType: asset.MIMEType, SHA256: asset.SHA256,
			Source: asset.Source, AltText: asset.AltText,
		})
	}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not encode package")
		return
	}
	if setPackageCacheHeaders(w, r, game, strongETag(encoded)) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func (s *Server) handleGetGameAsset(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "Game IR has not been published")
		return
	}
	assetID := r.PathValue("asset")
	for _, asset := range game.GameIR.Assets {
		if asset.ID != assetID {
			continue
		}
		data, contentType, found := s.store.GetArtifact(game.ID, asset.URI)
		if !found {
			writeErr(w, http.StatusNotFound, "asset content not found")
			return
		}
		if contentType == "" {
			contentType = asset.MIMEType
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if setPackageCacheHeaders(w, r, game, strongETag(data)) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	writeErr(w, http.StatusNotFound, "asset not found in Game IR")
}
