// Package api is the HTTP layer: JSON endpoints for ingest, games, jobs,
// groups, sharing, and bundle download.
package api

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/socialgraph"
	"doodlequest/internal/store"
	_ "github.com/lib/pq"
)

//go:embed web/player.html
var playerHTML string

//go:embed web/pixi-runtime.js
var pixiRuntimeJS string

// Vendored engine builds (auto detect-and-fix flow, phase 0): the player
// loads PixiJS and Rapier from this backend, never from a CDN re-bundle —
// jsDelivr's +esm build of pixi.js drops side-effect init registrations
// (filter/text render pipes), which blanked the stage in production.
//
//go:embed web/vendor/pixi.min.mjs
var pixiVendorJS string

//go:embed web/vendor/rapier.es.js
var rapierVendorJS string

type Server struct {
	store               store.Store
	runner              *pipeline.Runner
	deliveryMu          sync.Mutex
	deliveryMetrics     map[string]*deliveryMetricSnapshot
	deliveryInvalidator DeliveryCacheInvalidator
	previewScanner      PreviewScanner
	socialMu            sync.RWMutex
	socialGraph         socialgraph.Store
	socialGraphErr      error
	follows             map[string]map[string]struct{}
	reactions           map[string]map[string]string
	comments            map[string][]socialComment
	saves               map[string]map[string]bool
	blocks              map[string]map[string]bool
	notInterested       map[string]map[string]bool
	mutedTopics         map[string]map[string]bool
	profiles            map[string]creatorProfile
	commentIdempotency  map[string]commentIdempotencyRecord
	// deletedSubjects tombstones account-deleted subjects: their bearer
	// tokens stop working and they cannot sign in again (see apple_auth.go).
	deletedSubjects map[string]bool
	reports         []socialReport
	events          []socialEvent

	// Phase 2 telemetry index: every runtime-error report this process has
	// accepted (plus the JSONL backlog, loaded lazily), used for incident
	// aggregation and the auto-quarantine thresholds.
	teleMu     sync.Mutex
	teleEvents []teleEvent
	teleLoaded bool
	// Telemetry v2 indexes (Phase A): session starts are the
	// denominator for rate-based quarantine; perf windows / runs /
	// milestones feed GET /v1/telemetry/perf.
	teleSessions []teleSession
	telePerf     []telePerfWin
	teleRuns     []teleRun
	teleMiles    []teleMile
	// Bounded process-lifetime deduplication for client telemetry retries.
	teleV2EventIDs   map[string]struct{}
	teleV2EventOrder []string

	// Phase 3 shadow-mode repair proposals, keyed by game id. In-memory
	// only: a proposal is a suggestion until a human applies it.
	repairMu        sync.Mutex
	repairProposals map[string]*repairProposal

	// Leaderboard v2 (Phase C): submitted runs with input traces,
	// persisted to runs.jsonl under the telemetry dir and indexed in
	// memory; boards are derived per (game, IR hash).
	runsMu     sync.Mutex
	runs       []RunRecord
	runsLoaded bool
	runSeq     int

	// Difficulty tuner (Phase E): shadow pacing proposals keyed by
	// proposal id. In-memory only; nothing applies itself.
	tunerMu        sync.Mutex
	tunerProposals map[string]*tunerProposal
	tunerSeq       int
}

func New(st store.Store, r *pipeline.Runner) *Server {
	s := &Server{store: st, runner: r, deliveryInvalidator: noopDeliveryCacheInvalidator{}, follows: map[string]map[string]struct{}{}, reactions: map[string]map[string]string{}, comments: map[string][]socialComment{}, saves: map[string]map[string]bool{}, blocks: map[string]map[string]bool{}, notInterested: map[string]map[string]bool{}, mutedTopics: map[string]map[string]bool{}, profiles: map[string]creatorProfile{}, commentIdempotency: map[string]commentIdempotencyRecord{}, deletedSubjects: map[string]bool{}, teleV2EventIDs: map[string]struct{}{}}
	if dsn := strings.TrimSpace(os.Getenv("DQ_SOCIAL_DATABASE_URL")); dsn != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		graph, err := socialgraph.OpenPostgres(ctx, dsn)
		cancel()
		if err != nil {
			s.socialGraphErr = err
			log.Printf("shared social graph unavailable: %v", err)
		} else {
			s.socialGraph = graph
		}
	}
	if err := pruneTelemetryOnStartup(); err != nil {
		log.Printf("telemetry retention skipped: %v", err)
	}
	s.loadSocialState()
	// Load the telemetry backlog once, up front: after this, the index is
	// fed only by newly accepted reports, so a report is never counted
	// twice (once from the file, once from memory).
	s.teleMu.Lock()
	s.loadTeleIndexLocked()
	s.loadTeleV2EventIDsLocked()
	s.loadTeleV2IndexesLocked()
	s.teleMu.Unlock()
	s.runsMu.Lock()
	s.loadRunsLocked()
	s.runsMu.Unlock()
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReadiness)
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /player", s.handlePlayer)
	mux.HandleFunc("GET /web/pixi-runtime.js", s.handlePixiRuntime)
	mux.HandleFunc("GET /web/vendor/pixi.min.mjs", s.handlePixiVendor)
	mux.HandleFunc("GET /web/vendor/rapier.es.js", s.handleRapierVendor)
	mux.HandleFunc("POST /v1/auth/apple", s.handleAppleSignIn)
	mux.HandleFunc("DELETE /v1/users/me", s.handleDeleteAccount)
	mux.HandleFunc("POST /v1/telemetry/runtime-error", s.handleRuntimeError)
	mux.HandleFunc("POST /v1/telemetry/events", s.handleTelemetryEvent)
	mux.HandleFunc("GET /v1/telemetry/perf", s.handlePerfSummary)
	mux.HandleFunc("GET /v1/telemetry/delivery", s.handleDeliveryMetrics)
	mux.HandleFunc("GET /v1/telemetry/incidents", s.handleIncidents)
	mux.HandleFunc("POST /v1/games/{id}/quarantine", s.handleQuarantine)
	mux.HandleFunc("POST /v1/games/{id}/unquarantine", s.handleUnquarantine)
	mux.HandleFunc("POST /v1/games/{id}/tuner/propose", s.tunerPropose)
	mux.HandleFunc("GET /v1/games/{id}/tuner/proposals", s.tunerList)
	mux.HandleFunc("POST /v1/games/{id}/tuner/proposals/{pid}/apply", func(w http.ResponseWriter, r *http.Request) { s.tunerResolve(w, r, true) })
	mux.HandleFunc("POST /v1/games/{id}/tuner/proposals/{pid}/discard", func(w http.ResponseWriter, r *http.Request) { s.tunerResolve(w, r, false) })
	mux.HandleFunc("POST /v1/games/{id}/repair/propose", s.handleRepairPropose)
	mux.HandleFunc("GET /v1/games/{id}/repair/proposal", s.handleRepairProposal)
	mux.HandleFunc("POST /v1/games/{id}/repair/apply", s.handleRepairApply)
	mux.HandleFunc("POST /v1/games/{id}/repair/discard", s.handleRepairDiscard)
	mux.HandleFunc("POST /v1/ingest", s.handleIngest)
	mux.HandleFunc("POST /v1/games", s.handleCreateGame)
	mux.HandleFunc("GET /v1/games/{id}", s.handleGetGame)
	mux.HandleFunc("PUT /v1/games/{id}/previews/{version}/{asset}", s.handleUploadPreviewAsset)
	mux.HandleFunc("GET /v1/games/{id}/previews/{version}/{asset}", s.handleGetPreviewAsset)
	mux.HandleFunc("PUT /v1/games/{id}/preview", s.handleSetPreviewMetadata)
	mux.HandleFunc("POST /v1/games/{id}/confirm", s.handleConfirm)
	mux.HandleFunc("GET /v1/games/{id}/bundle", s.handleGetBundle)
	mux.HandleFunc("GET /v1/games/{id}/bundle/game.js", s.handleGameJS)
	mux.HandleFunc("GET /v1/games/{id}/bundle/sprites/{sprite}", s.handleSprite)
	mux.HandleFunc("GET /v1/games/{id}/bundle/audio/theme.wav", s.handleMusic)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /v1/groups", s.handleCreateGroup)
	mux.HandleFunc("POST /v1/groups/{id}/join", s.handleJoinGroup)
	mux.HandleFunc("GET /v1/groups/{id}/games", s.handleGroupGames)
	mux.HandleFunc("POST /v1/games/{id}/visibility", s.handleVisibility)
	mux.HandleFunc("GET /v1/feed", s.handleFeed)
	s.registerSocialRoutes(mux)
	mux.HandleFunc("POST /v1/games/{id}/runs", s.handleSubmitRun)
	mux.HandleFunc("GET /v1/games/{id}/leaderboard", s.handleLeaderboardV2)
	mux.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /v1/runs/{id}/verification", s.handleRunVerification)
	mux.HandleFunc("POST /v1/spec/decide", s.handleSpecDecide)
	mux.HandleFunc("GET /v1/spec/questionnaire", s.handleSpecQuestionnaire)
	mux.HandleFunc("POST /v1/ir/validate", s.handleIRValidate)
	mux.HandleFunc("POST /v1/game-ir/validate", s.handleGameIRValidate)
	mux.HandleFunc("GET /v1/games/{id}/game-ir", s.handleGetGameIR)
	mux.Handle("GET /v1/games/{id}/assets/{asset}", s.observeDelivery("asset", s.handleGetGameAsset))
	mux.Handle("GET /v1/games/{id}/package", s.observeDelivery("package", s.handleGetPackage))
	return cors(s.requireAuthenticatedUser(mux))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-User-ID, X-Moderation-Token, X-Telemetry-Admin-Token, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireModerationToken gates operator-only endpoints (manual
// quarantine, repair apply, tuner apply) behind the same credential
// the social moderation handlers use: the DQ_MODERATION_TOKEN
// environment variable, presented as the X-Moderation-Token header.
// Fails closed — unconfigured means 503, never open. Before this
// check existed those endpoints accepted anyone (architecture
// review 2026-10-23); propose/dismiss stay open because they change
// nothing live.
func requireModerationToken(w http.ResponseWriter, r *http.Request) bool {
	want := strings.TrimSpace(os.Getenv("DQ_MODERATION_TOKEN"))
	if want == "" {
		writeErr(w, http.StatusServiceUnavailable, "operator actions are not configured; set DQ_MODERATION_TOKEN")
		return false
	}
	if r.Header.Get("X-Moderation-Token") != want {
		writeErr(w, http.StatusForbidden, "invalid moderation token")
		return false
	}
	return true
}

// userID returns the verified bearer <redacted> in production (Sign in with Apple
// subjects are namespaced as apple:<sub>). Legacy identity headers are only
// honored when explicitly enabled for local development.
func userID(r *http.Request) string {
	if subject, ok := r.Context().Value(authContextKey{}).(string); ok && subject != "" {
		return subject
	}
	// Legacy identity headers are only honored when explicitly enabled for local development.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DQ_ALLOW_INSECURE_USER_ID")), "true") {
		if u := strings.TrimSpace(r.Header.Get("X-User-ID")); u != "" {
			return u
		}
		return "demo-parent"
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

const maxJSONBodyBytes = 2 << 20 // 2 MiB

// maxPhotoBodyBytes caps the two photo-carrying endpoints. A phone
// photo sent as a base64 data URL runs 3–8 MiB; the shared 2 MiB cap
// rejected real uploads outright (architecture review 2026-10-23).
// The durable fix is on-device downscale + multipart; until then the
// create/ingest routes accept up to 16 MiB while every other route
// keeps the tight cap.
const maxPhotoBodyBytes = 16 << 20 // 16 MiB

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONLimit(w, r, v, maxJSONBodyBytes)
}

func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if r.Body == nil {
		writeErr(w, http.StatusBadRequest, "request body is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body exceeds the size limit")
		} else {
			writeErr(w, http.StatusBadRequest, "invalid JSON request body")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeErr(w, http.StatusBadRequest, "request body must contain exactly one JSON value")
		return false
	}
	return true
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "doodlequest"})
}

// handleReadiness reports whether configured durable state can currently be
// loaded and written. Liveness remains available through /healthz.
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{"status": "ok", "service": "doodlequest"}
	if s.socialGraphErr != nil {
		status["status"] = "degraded"
		status["shared_social_graph"] = false
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}
	if s.socialGraph != nil {
		status["shared_social_graph"] = true
	}
	if persistence, ok := s.store.(interface{ PersistenceStatus() (bool, bool) }); ok {
		enabled, healthy := persistence.PersistenceStatus()
		status["persistence_enabled"] = enabled
		if enabled && !healthy {
			status["status"] = "degraded"
			writeJSON(w, http.StatusServiceUnavailable, status)
			return
		}
	}
	writeJSON(w, http.StatusOK, status)
}

// GET / — send browsers straight to the player.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/player", http.StatusFound)
}

// GET /player — the web player: lists public games, runs any bundle
// through the GameShim v1 runtime.
func (s *Server) handlePlayer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(playerHTML))
}

// GET /web/pixi-runtime.js — the PixiJS + Rapier Game IR runtime module
// loaded by the player page (single runtime since 2026-10-20).
func (s *Server) handlePixiRuntime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(pixiRuntimeJS))
}

// GET /web/vendor/pixi.min.mjs — vendored PixiJS 8.22.0 official dist build.
func (s *Server) handlePixiVendor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(pixiVendorJS))
}

// GET /web/vendor/rapier.es.js — vendored Rapier 0.15.1 compat ESM build.
func (s *Server) handleRapierVendor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(rapierVendorJS))
}

// POST /v1/ingest — Stage 0: drawing + keywords -> elements + guessed intent.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	var req model.IngestRequest
	if !decodeJSONLimit(w, r, &req, maxPhotoBodyBytes) {
		return
	}
	elements, intent := pipeline.StubVisionAnalyze(req.Images, req.Keywords)
	res := &model.IngestResult{
		IngestID:      pipeline.NewID(),
		Elements:      elements,
		GuessedIntent: intent,
		Keywords:      req.Keywords,
		CreatedAt:     time.Now(),
	}
	s.store.CreateIngest(res)
	writeJSON(w, http.StatusOK, res)
}

// POST /v1/games — create a game; SPEC runs async, then pauses for confirm.
func (s *Server) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	var req model.CreateGameRequest
	if !decodeJSONLimit(w, r, &req, maxPhotoBodyBytes) {
		return
	}
	elements, intent := req.Elements, req.Intent
	if req.ParentGameID != "" {
		parent, ok := s.store.GetGame(req.ParentGameID)
		if !ok || parent.Status != model.GameReady || parent.Manifest == nil {
			writeErr(w, http.StatusBadRequest, "parent_game_id must identify a ready inventory game")
			return
		}
		if len(elements) == 0 {
			elements = append([]model.Element(nil), parent.Elements...)
		}
		if intent == "" {
			intent = parent.Intent
		}
	}
	if req.IngestID != "" {
		if ing, ok := s.store.GetIngest(req.IngestID); ok {
			if len(elements) == 0 {
				elements = ing.Elements
			}
			if intent == "" {
				intent = ing.GuessedIntent
			}
		} else {
			writeErr(w, http.StatusBadRequest, "unknown ingest_id")
			return
		}
	}
	if len(elements) == 0 {
		writeErr(w, http.StatusBadRequest, "need elements (or a valid ingest_id)")
		return
	}
	now := time.Now()
	game := &model.Game{
		ID:           pipeline.NewID(),
		ParentGameID: req.ParentGameID,
		Title:        "Untitled Game",
		OwnerID:      userID(r),
		Elements:     elements,
		Intent:       intent,
		Visibility:   model.VisibilityPrivate,
		Status:       model.GameGenerating,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.store.CreateGame(game)
	job := s.runner.CreateAndStart(game, elements, intent, req.Keywords)
	game.JobID = job.ID
	s.store.UpdateGame(game)
	writeJSON(w, http.StatusAccepted, map[string]any{"game": game, "job": job})
}

func (s *Server) handleGetGame(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, game)
}

// POST /v1/games/{id}/confirm — approve (optionally edited) design doc,
// resuming art -> codegen -> validate -> publish.
func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var req model.ConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	job, err := s.runner.Confirm(r.PathValue("id"), req.DesignDoc)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.store.GetJob(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetBundle(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	// GameShim is transitional. Never let shared caches retain its bytes
	// across visibility or moderation changes.
	w.Header().Set("Cache-Control", "private, no-store")
	if game.Manifest == nil {
		writeErr(w, http.StatusConflict, "bundle not published yet")
		return
	}
	writeJSON(w, http.StatusOK, game.Manifest)
}

func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request, name string) {
	if _, ok := s.deliverableGame(w, r); !ok {
		return
	}
	// Legacy bundle artifacts have no immutable version URL; disable caching.
	w.Header().Set("Cache-Control", "private, no-store")
	data, ct, ok := s.store.GetArtifact(r.PathValue("id"), name)
	if !ok {
		w.Header().Set("Cache-Control", "private, no-store")
		writeErr(w, http.StatusNotFound, "artifact not found")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleGameJS(w http.ResponseWriter, r *http.Request) {
	s.serveArtifact(w, r, "game.js")
}

func (s *Server) handleSprite(w http.ResponseWriter, r *http.Request) {
	s.serveArtifact(w, r, "sprites/"+r.PathValue("sprite"))
}

func (s *Server) handleMusic(w http.ResponseWriter, r *http.Request) {
	s.serveArtifact(w, r, "audio/theme.wav")
}

// POST /v1/groups — create an invite-based group.
func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req model.CreateGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	u := userID(r)
	gr := &model.Group{
		ID:         pipeline.NewID(),
		Name:       req.Name,
		OwnerID:    u,
		InviteCode: strings.ToUpper(pipeline.NewID()[:6]),
		MemberIDs:  []string{u},
		CreatedAt:  time.Now(),
	}
	s.store.CreateGroup(gr)
	writeJSON(w, http.StatusCreated, gr)
}

// POST /v1/groups/{id}/join — join via invite code.
func (s *Server) handleJoinGroup(w http.ResponseWriter, r *http.Request) {
	var req model.JoinGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	gr, ok := s.store.GetGroup(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "group not found")
		return
	}
	if !strings.EqualFold(gr.InviteCode, strings.TrimSpace(req.InviteCode)) {
		writeErr(w, http.StatusForbidden, "bad invite code")
		return
	}
	u := userID(r)
	for _, m := range gr.MemberIDs {
		if m == u {
			writeJSON(w, http.StatusOK, gr)
			return
		}
	}
	gr.MemberIDs = append(gr.MemberIDs, u)
	s.store.UpdateGroup(gr)
	writeJSON(w, http.StatusOK, gr)
}

// GET /v1/groups/{id}/games — games the author shared with this group.
func (s *Server) handleGroupGames(w http.ResponseWriter, r *http.Request) {
	gr, ok := s.store.GetGroup(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "group not found")
		return
	}
	u := userID(r)
	member := false
	for _, m := range gr.MemberIDs {
		if m == u {
			member = true
			break
		}
	}
	if !member {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}
	var out []*model.Game
	for _, g := range s.store.ListGames() {
		if g.Status != model.GameReady {
			continue
		}
		if g.Visibility == model.VisibilityGroups {
			for _, gid := range g.GroupIDs {
				if gid == gr.ID {
					out = append(out, g)
				}
			}
		}
	}
	if out == nil {
		out = []*model.Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// POST /v1/games/{id}/visibility — author-controlled sharing.
func (s *Server) handleVisibility(w http.ResponseWriter, r *http.Request) {
	var req model.VisibilityRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.OwnerID != userID(r) {
		writeErr(w, http.StatusForbidden, "only the author can change visibility")
		return
	}
	switch req.Visibility {
	case model.VisibilityPrivate, model.VisibilityGroups, model.VisibilityPublic:
	default:
		writeErr(w, http.StatusBadRequest, "visibility must be private, groups, or public")
		return
	}
	game.Visibility = req.Visibility
	game.GroupIDs = req.GroupIDs
	if game.Manifest != nil {
		game.Manifest.Visibility = req.Visibility
		game.Manifest.SharedGroupIDs = req.GroupIDs
	}
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
	s.invalidateDeliveryCache(r.Context(), game.ID)
	writeJSON(w, http.StatusOK, game)
}

// loadAllIDs returns the viewer's full internal filter set. When the store
// implements socialgraph.AllIDStore it uses the single call; otherwise it
// builds the same set from the individual List* methods, with the same
// fail-fast semantics: any failing set aborts the whole load, so a caller
// never acts on a partially loaded filter set.
func (s *Server) loadAllIDs(ctx context.Context, viewer string) (socialgraph.AllIDs, error) {
	if store, ok := s.socialGraph.(socialgraph.AllIDStore); ok {
		return store.ListAllIDs(ctx, viewer)
	}
	var all socialgraph.AllIDs
	var err error
	if all.Following, err = s.socialGraph.ListFollowing(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.Followers, err = s.socialGraph.ListFollowers(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.Blocked, err = s.socialGraph.ListBlocked(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.Blockers, err = s.socialGraph.ListBlockers(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.SavedGames, err = s.socialGraph.ListSavedGames(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.NotInterested, err = s.socialGraph.ListNotInterested(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	if all.MutedTopics, err = s.socialGraph.ListMutedTopics(ctx, viewer); err != nil {
		return socialgraph.AllIDs{}, err
	}
	return all, nil
}

// GET /v1/feed?scope=for_you|following|friends. This is a transparent in-memory
// baseline; personalized ranking can evolve after event volume is meaningful.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "for_you"
	}
	if scope != "for_you" && scope != "following" && scope != "friends" {
		writeErr(w, 400, "scope must be for_you, following, or friends")
		return
	}
	viewer := userID(r)
	if s.socialGraphErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "shared social graph is unavailable")
		return
	}
	s.socialMu.RLock()
	followed := map[string]bool{}
	followers := map[string]bool{}
	if s.socialGraph == nil {
		for id := range s.follows[viewer] {
			followed[id] = true
		}
		if scope == "friends" {
			for id := range s.follows {
				if _, ok := s.follows[id][viewer]; ok {
					followers[id] = true
				}
			}
		}
	}
	mutual := map[string]bool{}
	blockedIDs := []string{}
	blockerIDs := []string{}
	ignoredIDs := []string{}
	mutedTopicIDs := []string{}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		all, err := s.loadAllIDs(ctx, viewer)
		cancel()
		if err != nil {
			s.socialMu.RUnlock()
			writeErr(w, http.StatusServiceUnavailable, "could not read shared social graph")
			return
		}
		for _, id := range all.Following {
			followed[id] = true
		}
		blockedIDs = all.Blocked
		blockerIDs = all.Blockers
		ignoredIDs = all.NotInterested
		mutedTopicIDs = all.MutedTopics
		if scope == "friends" {
			for _, id := range all.Followers {
				followers[id] = true
			}
		}
	}
	if scope == "friends" {
		for id := range followed {
			if followers[id] {
				mutual[id] = true
			}
		}
	}
	blocked := map[string]bool{}
	if s.socialGraph != nil {
		for _, id := range blockedIDs {
			blocked[id] = true
		}
		for _, id := range blockerIDs {
			blocked[id] = true
		}
	} else {
		for id := range s.blocks[viewer] {
			blocked[id] = true
		}
		for creator, users := range s.blocks {
			if users[viewer] {
				blocked[creator] = true
			}
		}
	}
	ignored := map[string]bool{}
	if s.socialGraph != nil {
		for _, id := range ignoredIDs {
			ignored[id] = true
		}
	} else {
		for id := range s.notInterested[viewer] {
			ignored[id] = true
		}
	}
	mutedTopics := map[string]bool{}
	if s.socialGraph != nil {
		for _, topic := range mutedTopicIDs {
			mutedTopics[strings.ToLower(topic)] = true
		}
	} else {
		for topic, muted := range s.mutedTopics[viewer] {
			if muted {
				mutedTopics[strings.ToLower(topic)] = true
			}
		}
	}
	events := append([]socialEvent(nil), s.events...)
	reactionCounts := map[string]int{}
	for id, users := range s.reactions {
		reactionCounts[id] = len(users)
	}
	s.socialMu.RUnlock()
	var out []*model.Game
	for _, g := range s.store.ListGames() {
		if g.Status != model.GameReady || g.Visibility != model.VisibilityPublic || g.Quarantined || g.ModerationStatus == "pending" || g.ModerationStatus == "rejected" || blocked[g.OwnerID] || ignored[g.ID] {
			continue
		}
		topicMuted := false
		for _, topic := range g.Topics {
			if mutedTopics[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(topic, "#")))] {
				topicMuted = true
				break
			}
		}
		if topicMuted {
			continue
		}
		if scope == "following" && !followed[g.OwnerID] {
			continue
		}
		if scope == "friends" && !mutual[g.OwnerID] {
			continue
		}
		out = append(out, g)
	}
	if s.socialGraph != nil {
		gameIDs := make([]string, 0, len(out))
		for _, game := range out {
			gameIDs = append(gameIDs, game.ID)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		counts, _, err := s.socialGraph.ListReactionSummary(ctx, viewer, gameIDs)
		cancel()
		if err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not read shared reactions")
			return
		}
		reactionCounts = counts
	}
	// Keep the ranking explainable and avoid treating a handful of early
	// interactions as a reliable quality signal. Until at least 20 social
	// events/reactions exist in the current index, use the newest-first
	// baseline. This threshold is a guardrail, not a learned parameter.
	signalCount := 0
	for _, event := range events {
		// Player diagnostics are useful observability but are not engagement
		// evidence and must not activate the quality ranker by themselves.
		if event.Kind != "player_ready" && event.Kind != "player_session_summary" {
			signalCount++
		}
	}
	for _, count := range reactionCounts {
		signalCount += count
	}
	useEngagementRanking := engagementRankingEnabled() && signalCount >= 20
	// Aggregate ranking features once per request; the existing flag remains
	// the immediate rollback to newest-first.
	engagementScores := feedEngagementByGame(events)
	qualitySignals := feedQualitySignalsByGame(events)
	sort.SliceStable(out, func(i, j int) bool {
		if !useEngagementRanking {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		si := feedQualityScore(out[i], reactionCounts, engagementScores, qualitySignals[out[i].ID])
		sj := feedQualityScore(out[j], reactionCounts, engagementScores, qualitySignals[out[j].ID])
		if si == sj {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return si > sj
	})
	if useEngagementRanking {
		out = reserveExplorationSlots(out, qualitySignals, 5)
	}
	out = diversifyFeed(out)
	if out == nil {
		out = []*model.Game{}
	}
	page, next, err := paginateGames(out, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cards, err := s.buildFeedCards(r.Context(), page, viewer, followed)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not build viewer feed state")
		return
	}
	writeJSON(w, 200, map[string]any{
		"games": page, "scope": scope, "next_cursor": next,
		"card_schema_version": FeedCardSchemaVersion, "cards": cards,
	})
}

func diversifyFeed(ranked []*model.Game) []*model.Game {
	// Round-robin creator groups in score order to avoid a single prolific
	// creator filling the entire feed. Each creator's internal score order stays intact.
	groups := map[string][]*model.Game{}
	creatorOrder := []string{}
	for _, g := range ranked {
		creator := g.OwnerID
		if creator == "" {
			creator = "unknown:" + g.ID
		}
		if _, exists := groups[creator]; !exists {
			creatorOrder = append(creatorOrder, creator)
		}
		groups[creator] = append(groups[creator], g)
	}
	out := make([]*model.Game, 0, len(ranked))
	for len(out) < len(ranked) {
		progress := false
		for _, creator := range creatorOrder {
			if len(groups[creator]) == 0 {
				continue
			}
			out = append(out, groups[creator][0])
			groups[creator] = groups[creator][1:]
			progress = true
		}
		if !progress {
			break
		}
	}
	return out
}
func paginateGames(games []*model.Game, r *http.Request) ([]*model.Game, string, error) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			return nil, "", fmt.Errorf("limit must be between 1 and 50")
		}
		limit = n
	}
	start := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		found := false
		for i, g := range games {
			if g.ID == cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return nil, "", fmt.Errorf("cursor does not match an item in this feed")
		}
	}
	if start >= len(games) {
		return []*model.Game{}, "", nil
	}
	end := start + limit
	if end > len(games) {
		end = len(games)
	}
	page := games[start:end]
	next := ""
	if end < len(games) && len(page) > 0 {
		next = page[len(page)-1].ID
	}
	return page, next, nil
}

// engagementRankingEnabled provides an immediate rollback switch while the
// ranking policy is evaluated against real engagement and diversity metrics.
// Ranking remains enabled by default, with the newest-first cold-start fallback.
func engagementRankingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DQ_FEED_RANKING_ENABLED"))) {
	case "false", "0", "off", "disabled":
		return false
	default:
		return true
	}
}

func feedEngagementByGame(events []socialEvent) map[string]float64 {
	// Keep the current explainable weights in one pass over the event window.
	// Unknown event kinds intentionally contribute nothing to ranking.
	weights := map[string]float64{
		"completion": 2,
		"replay":     1.5,
		"share":      1.25,
		"save":       1.25,
		"quick_exit": -0.5,
	}
	byGame := make(map[string]float64)
	for _, event := range events {
		if weight, ok := weights[event.Kind]; ok && event.GameID != "" {
			byGame[event.GameID] += weight
		}
	}
	return byGame
}

// feedQualitySignals stores bounded, per-game rates rather than allowing a
// single popular game or a burst of duplicate events to dominate by raw count.
type feedQualitySignals struct {
	Impressions            int
	Plays                  int
	Completions            int
	Replays                int
	QuickExits             int
	CompletionQuality      float64
	ReplayQuality          float64
	AvgTimeToInteractiveMS float64
	TTISamples             int
}

func feedQualitySignalsByGame(events []socialEvent) map[string]feedQualitySignals {
	out := make(map[string]feedQualitySignals)
	for _, event := range events {
		if event.GameID == "" {
			continue
		}
		s := out[event.GameID]
		switch event.Kind {
		case "impression":
			s.Impressions++
		case "play_start":
			s.Plays++
		case "completion":
			s.Completions++
			if event.ExpectedMS > 0 && event.DurationMS > 0 {
				ratio := float64(event.DurationMS) / float64(event.ExpectedMS)
				if ratio > 1 {
					ratio = 1
				}
				s.CompletionQuality += ratio
			}
		case "replay":
			s.Replays++
			if event.ExpectedMS > 0 && event.DurationMS > 0 {
				ratio := float64(event.DurationMS) / float64(event.ExpectedMS)
				if ratio > 1 {
					ratio = 1
				}
				s.ReplayQuality += ratio
			}
		case "quick_exit":
			s.QuickExits++
		}
		if event.TimeToInteractiveMS > 0 {
			s.AvgTimeToInteractiveMS += float64(event.TimeToInteractiveMS)
			s.TTISamples++
		}
		out[event.GameID] = s
	}
	for id, s := range out {
		if s.TTISamples > 0 {
			s.AvgTimeToInteractiveMS /= float64(s.TTISamples)
		}
		out[id] = s
	}
	return out
}

func feedQualityScore(g *model.Game, reactions map[string]int, engagement map[string]float64, s feedQualitySignals) float64 {
	score := feedScore(g, reactions, engagement)
	// Rates are smoothed so a single early event does not create a perfect score.
	tapRate := float64(s.Plays) / float64(s.Impressions+3)
	completionRate := float64(s.Completions) / float64(s.Plays+3)
	replayRate := float64(s.Replays) / float64(s.Completions+3)
	quickExitRate := float64(s.QuickExits) / float64(s.Plays+3)
	completionQuality := s.CompletionQuality / float64(s.Completions+1)
	replayQuality := s.ReplayQuality / float64(s.Replays+1)
	score += 0.20*tapRate + 0.18*completionRate*completionQuality + 0.10*replayRate*replayQuality
	score -= 0.18 * quickExitRate
	if s.TTISamples > 0 {
		// No penalty below 1.5s; a bounded penalty ramps up to 0.15 at 6s.
		penalty := (s.AvgTimeToInteractiveMS - 1500) / 30000
		if penalty < 0 {
			penalty = 0
		}
		if penalty > 0.15 {
			penalty = 0.15
		}
		score -= penalty
	}
	return score
}

// reserveExplorationSlots promotes a recent low-impression candidate into
// every fifth position, retaining the ranked order of both candidate pools.
func reserveExplorationSlots(ranked []*model.Game, signals map[string]feedQualitySignals, interval int) []*model.Game {
	if interval < 2 || len(ranked) < interval {
		return ranked
	}
	explore := make([]*model.Game, 0)
	exploit := make([]*model.Game, 0, len(ranked))
	for _, game := range ranked {
		s := signals[game.ID]
		if s.Impressions < 3 {
			explore = append(explore, game)
		} else {
			exploit = append(exploit, game)
		}
	}
	if len(explore) == 0 || len(exploit) == 0 {
		return ranked
	}
	out := make([]*model.Game, 0, len(ranked))
	for len(explore) > 0 || len(exploit) > 0 {
		for i := 1; i < interval && len(exploit) > 0; i++ {
			out = append(out, exploit[0])
			exploit = exploit[1:]
		}
		if len(explore) > 0 {
			out = append(out, explore[0])
			explore = explore[1:]
		}
		if len(exploit) == 0 && len(explore) > 0 {
			out = append(out, explore...)
			break
		}
		if len(explore) == 0 && len(exploit) > 0 {
			out = append(out, exploit...)
			break
		}
	}
	return out
}

func feedScore(g *model.Game, reactions map[string]int, engagement map[string]float64) float64 {
	age := time.Since(g.CreatedAt).Hours()
	if age < 0 {
		age = 0
	}
	fresh := 1 / (1 + age/24)
	eng := float64(reactions[g.ID]) + engagement[g.ID]
	return fresh + 0.03*eng
}
