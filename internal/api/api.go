// Package api is the HTTP layer: JSON endpoints for ingest, games, jobs,
// groups, sharing, and bundle download.
package api

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

//go:embed web/player.html
var playerHTML string

type Server struct {
	store  store.Store
	runner *pipeline.Runner
}

func New(st store.Store, r *pipeline.Runner) *Server {
	return &Server{store: st, runner: r}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /player", s.handlePlayer)
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
	mux.HandleFunc("POST /v1/games/{id}/scores", s.handleSubmitScore)
	mux.HandleFunc("GET /v1/games/{id}/leaderboard", s.handleLeaderboard)
	mux.HandleFunc("GET /v1/godot", s.handleGodotList)
	mux.HandleFunc("GET /godot/", s.handleGodot)
	mux.HandleFunc("POST /v1/spec/decide", s.handleSpecDecide)
	mux.HandleFunc("GET /v1/spec/questionnaire", s.handleSpecQuestionnaire)
	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
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

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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
		ID:         pipeline.NewID(),
		Title:      "Untitled Game",
		OwnerID:    userID(r),
		Elements:   elements,
		Intent:     intent,
		Visibility: model.VisibilityPrivate,
		Status:     model.GameGenerating,
		CreatedAt:  now,
		UpdatedAt:  now,
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

// GET /v1/feed — public, ready games.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	var out []*model.Game
	for _, g := range s.store.ListGames() {
		if g.Status == model.GameReady && g.Visibility == model.VisibilityPublic {
			out = append(out, g)
		}
	}
	if out == nil {
		out = []*model.Game{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// POST /v1/games/{id}/scores — record a run on the leaderboard.
// Keeps each player's best run (ranked by level reached, then score).
func (s *Server) handleSubmitScore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
		Level int    `json:"level"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	if _, ok := s.store.GetGame(id); !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Player"
	}
	if len(name) > 24 {
		name = name[:24]
	}
	if req.Level < 1 {
		req.Level = 1
	}
	if req.Score < 0 {
		req.Score = 0
	}
	rank := s.store.SubmitScore(id, name, req.Score, req.Level)
	writeJSON(w, http.StatusOK, map[string]any{"rank": rank, "name": name})
}

// GET /v1/games/{id}/leaderboard — top runs, best first.
func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.store.GetGame(id); !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	entries := s.store.Leaderboard(id, 10)
	if entries == nil {
		entries = []model.ScoreEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scores": entries})
}
