// Package api is the HTTP layer: JSON endpoints for ingest, games, jobs,
// groups, sharing, and bundle download.
package api

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
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
	store  store.Store
	runner *pipeline.Runner

	// Social graph MVP. These indexes are process-local until the Store
	// interface is backed by durable persistence.
	socialMu sync.RWMutex
	follows  map[string]map[string]struct{} // follower -> followed creator IDs

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
}

func New(st store.Store, r *pipeline.Runner) *Server {
	s := &Server{
		store: st, runner: r,
		follows: make(map[string]map[string]struct{}),
	}
	// Load the telemetry backlog once, up front: after this, the index is
	// fed only by newly accepted reports, so a report is never counted
	// twice (once from the file, once from memory).
	s.teleMu.Lock()
	s.loadTeleIndexLocked()
	s.teleMu.Unlock()
	s.runsMu.Lock()
	s.loadRunsLocked()
	s.runsMu.Unlock()
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /player", s.handlePlayer)
	mux.HandleFunc("GET /web/pixi-runtime.js", s.handlePixiRuntime)
	mux.HandleFunc("GET /web/vendor/pixi.min.mjs", s.handlePixiVendor)
	mux.HandleFunc("GET /web/vendor/rapier.es.js", s.handleRapierVendor)
	mux.HandleFunc("POST /v1/telemetry/runtime-error", s.handleRuntimeError)
	mux.HandleFunc("POST /v1/telemetry/events", s.handleTelemetryEvent)
	mux.HandleFunc("GET /v1/telemetry/perf", s.handlePerfSummary)
	mux.HandleFunc("GET /v1/telemetry/incidents", s.handleIncidents)
	mux.HandleFunc("POST /v1/games/{id}/quarantine", s.handleQuarantine)
	mux.HandleFunc("POST /v1/games/{id}/unquarantine", s.handleUnquarantine)
	mux.HandleFunc("POST /v1/games/{id}/repair/propose", s.handleRepairPropose)
	mux.HandleFunc("GET /v1/games/{id}/repair/proposal", s.handleRepairProposal)
	mux.HandleFunc("POST /v1/games/{id}/repair/apply", s.handleRepairApply)
	mux.HandleFunc("POST /v1/games/{id}/repair/discard", s.handleRepairDiscard)
	mux.HandleFunc("POST /v1/ingest", s.handleIngest)
	mux.HandleFunc("POST /v1/games", s.handleCreateGame)
	mux.HandleFunc("GET /v1/games/{id}", s.handleGetGame)
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
	mux.HandleFunc("POST /v1/users/{id}/follow", s.handleFollow)
	mux.HandleFunc("DELETE /v1/users/{id}/follow", s.handleUnfollow)
	mux.HandleFunc("GET /v1/users/me/following", s.handleListFollowing)
	mux.HandleFunc("POST /v1/games/{id}/runs", s.handleSubmitRun)
	mux.HandleFunc("GET /v1/games/{id}/leaderboard", s.handleLeaderboardV2)
	mux.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /v1/runs/{id}/verification", s.handleRunVerification)
	mux.HandleFunc("POST /v1/spec/decide", s.handleSpecDecide)
	mux.HandleFunc("GET /v1/spec/questionnaire", s.handleSpecQuestionnaire)
	mux.HandleFunc("POST /v1/ir/validate", s.handleIRValidate)
	mux.HandleFunc("POST /v1/game-ir/validate", s.handleGameIRValidate)
	mux.HandleFunc("GET /v1/games/{id}/game-ir", s.handleGetGameIR)
	mux.HandleFunc("GET /v1/games/{id}/assets/{asset}", s.handleGetGameAsset)
	mux.HandleFunc("GET /v1/games/{id}/package", s.handleGetPackage)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// userID is a stub for Sign in with Apple; the parent account owns creations.
func userID(r *http.Request) string {
	if u := r.Header.Get("X-User-ID"); u != "" {
		return u
	}
	return "demo-parent"
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

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		writeErr(w, http.StatusBadRequest, "request body is required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body exceeds 2 MiB")
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
	if !decodeJSON(w, r, &req) {
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
	if !decodeJSON(w, r, &req) {
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
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
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
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.Manifest == nil {
		writeErr(w, http.StatusConflict, "bundle not published yet")
		return
	}
	writeJSON(w, http.StatusOK, game.Manifest)
}

func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request, name string) {
	data, ct, ok := s.store.GetArtifact(r.PathValue("id"), name)
	if !ok {
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
	writeJSON(w, http.StatusOK, game)
}

// GET /v1/feed?scope=for_you|following|friends.
// "friends" means mutual follows. Public eligibility and quarantine checks
// apply to every scope, so private/group games never leak through social feeds.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" || scope == "for_you" {
		writeJSON(w, http.StatusOK, map[string]any{"games": s.publicFeedGames()})
		return
	}
	if scope != "following" && scope != "friends" {
		writeErr(w, http.StatusBadRequest, "scope must be for_you, following, or friends")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": s.socialFeedGames(userID(r), scope)})
}

func (s *Server) publicFeedGames() []*model.Game {
	var out []*model.Game
	for _, g := range s.store.ListGames() {
		if g.Status == model.GameReady && g.Visibility == model.VisibilityPublic && !g.Quarantined {
			out = append(out, g)
		}
	}
	if out == nil {
		out = []*model.Game{}
	}
	return out
}

func (s *Server) socialFeedGames(viewerID, scope string) []*model.Game {
	s.socialMu.RLock()
	viewerFollows := s.follows[viewerID]
	followed := make(map[string]struct{}, len(viewerFollows))
	for id := range viewerFollows {
		followed[id] = struct{}{}
	}
	mutual := make(map[string]struct{})
	if scope == "friends" {
		for creatorID := range followed {
			if _, followsBack := s.follows[creatorID][viewerID]; followsBack {
				mutual[creatorID] = struct{}{}
			}
		}
	}
	s.socialMu.RUnlock()

	eligible := followed
	if scope == "friends" {
		eligible = mutual
	}
	var out []*model.Game
	for _, g := range s.store.ListGames() {
		if g.Status != model.GameReady || g.Visibility != model.VisibilityPublic || g.Quarantined {
			continue
		}
		if _, ok := eligible[g.OwnerID]; ok {
			out = append(out, g)
		}
	}
	if out == nil {
		out = []*model.Game{}
	}
	return out
}

// POST /v1/users/{id}/follow — create a follow edge. The demo X-User-ID
// identity is not authentication and must be replaced before production use.
func (s *Server) handleFollow(w http.ResponseWriter, r *http.Request) {
	followerID, creatorID := strings.TrimSpace(userID(r)), strings.TrimSpace(r.PathValue("id"))
	if creatorID == "" || creatorID == followerID {
		writeErr(w, http.StatusBadRequest, "a different creator ID is required")
		return
	}
	if !s.hasPublicCreator(creatorID) {
		writeErr(w, http.StatusNotFound, "creator not found")
		return
	}
	s.socialMu.Lock()
	if s.follows[followerID] == nil {
		s.follows[followerID] = make(map[string]struct{})
	}
	s.follows[followerID][creatorID] = struct{}{}
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"follower_id": followerID, "creator_id": creatorID, "following": true})
}

// GET /v1/users/me/following — IDs followed by the current demo identity.
func (s *Server) handleListFollowing(w http.ResponseWriter, r *http.Request) {
	viewerID := strings.TrimSpace(userID(r))
	s.socialMu.RLock()
	ids := make([]string, 0, len(s.follows[viewerID]))
	for id := range s.follows[viewerID] {
		ids = append(ids, id)
	}
	s.socialMu.RUnlock()
	sort.Strings(ids)
	writeJSON(w, http.StatusOK, map[string]any{"creator_ids": ids})
}

// DELETE /v1/users/{id}/follow — remove a follow edge.
func (s *Server) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	followerID, creatorID := strings.TrimSpace(userID(r)), strings.TrimSpace(r.PathValue("id"))
	if creatorID == "" || creatorID == followerID {
		writeErr(w, http.StatusBadRequest, "a different creator ID is required")
		return
	}
	s.socialMu.Lock()
	delete(s.follows[followerID], creatorID)
	s.socialMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"follower_id": followerID, "creator_id": creatorID, "following": false})
}

func (s *Server) hasPublicCreator(creatorID string) bool {
	for _, g := range s.store.ListGames() {
		if g.OwnerID == creatorID && g.Status == model.GameReady && g.Visibility == model.VisibilityPublic && !g.Quarantined {
			return true
		}
	}
	return false
}



