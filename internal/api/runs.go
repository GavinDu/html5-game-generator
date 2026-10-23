package api

// Leaderboard v2 (Phase C of the IR 1.2 rollout): runs, boards, and
// replay verification.
//
// A run is a finished play session submitted by the player runtime:
// final score, win/lose, duration, the run's seed, and the full
// step-stamped input trace. Because the runtime simulates at a fixed
// timestep from a seeded RNG, a run is reproducible: the server can
// re-serve the trace (GET /v1/runs/{id}) and a browser can replay it
// (?replay=<id>) and report whether the same score comes out — that
// verdict lands via POST /v1/runs/{id}/verification and is shown on
// the board as the run's verified flag.
//
// Boards are keyed by (game, IR content hash): any IR change starts
// a fresh board, so scores from an older build never pollute the new
// one. Windows: "week" (rolling 7 days, the default — matches the
// fused design's weekly-default decision) and "all".
//
// Interim identity: display names are free text (filtered, ≤24
// chars), boards are unverified until a replay verdict arrives. The
// Sign-in-with-Apple identity layer replaces names later without
// changing the run model.
//
// Submissions are plausibility-capped from the game's own economics
// (the same recycle/stream model the validator's balance check
// uses): a score the game cannot physically produce in the run's
// duration is rejected, so the cap — not trust — is the first line
// of defense, and replay verification is the second.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"doodlequest/internal/gameir"
)

// irHashOf identifies one exact IR document (12 hex chars of sha256
// over its canonical JSON encoding).
func irHashOf(doc *gameir.Document) string {
	if doc == nil {
		return ""
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}

type RunInput struct {
	Tick   int    `json:"tick"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

type RunRecord struct {
	ID         string     `json:"id"`
	GameID     string     `json:"gameId"`
	IRHash     string     `json:"irHash"`
	Name       string     `json:"name"`
	Score      int        `json:"score"`
	Won        bool       `json:"won"`
	DurationMs int        `json:"durationMs"`
	Seed       uint32     `json:"seed"`
	Inputs     []RunInput `json:"inputs,omitempty"`
	At         time.Time  `json:"at"`
	// Verified is nil while no replay verdict exists; the board
	// renders that as "unverified". First verdict wins.
	Verified    *bool `json:"verified,omitempty"`
	ActualScore *int  `json:"actualScore,omitempty"`
}

// loadRunsLocked reads the runs backlog into memory (called once
// from New, mirroring the telemetry index load).
func (s *Server) loadRunsLocked() {
	if s.runsLoaded {
		return
	}
	s.runsLoaded = true
	f, err := os.Open(filepath.Join(telemetryDir(), "runs.jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		var rec RunRecord
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.ID != "" {
			s.runs = append(s.runs, rec)
		}
	}
}

func (s *Server) appendRunFile(rec RunRecord) {
	if err := os.MkdirAll(telemetryDir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(telemetryDir(), "runs.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if raw, err := json.Marshal(rec); err == nil {
		_, _ = f.Write(append(raw, '\n'))
	}
}

var nameBlocklist = []string{
	"fuck", "shit", "bitch", "nigger", "nigga", "faggot", "whore",
	"slut", "penis", "vagina", "porn", "rape", "nazi", "hitler",
	"cunt", "dick", "cock", "pussy", "retard",
}

// sanitizeName trims, caps, strips markup/control characters, and
// replaces blocklisted names with "Player" (flagged as filtered).
func sanitizeName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '<' || r == '>' || r == '"' || r == '\'' || r == '`' {
			continue
		}
		b.WriteRune(r)
	}
	name = b.String()
	if runes := []rune(name); len(runes) > 24 {
		name = string(runes[:24])
	}
	if name == "" {
		return "Player", false
	}
	lower := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return -1
	}, name)
	for _, bad := range nameBlocklist {
		if strings.Contains(lower, bad) {
			return "Player", true
		}
	}
	return name, false
}

// maxPlausibleScore caps a submission using the game's own scoring
// economics: the fastest the game can award points (recycle or
// stream model, mirroring the validator's balance check) times the
// run's duration, with slack for burst variance. Games whose
// economics can't be modeled get a generous generic rate.
func maxPlausibleScore(doc *gameir.Document, durationMs int) int {
	durSec := float64(durationMs) / 1000.0
	if doc == nil {
		return int(durSec*25) + 10
	}
	maxPts := 0.0
	hasCollect := false
	for _, r := range doc.Rules {
		if r.When.Event != "collected" {
			continue
		}
		hasCollect = true
		for _, a := range r.Actions {
			if a.Type == "add_score" {
				if fv, ok := a.Value.(float64); ok && fv > maxPts {
					maxPts = fv
				}
			}
		}
	}
	if !hasCollect || maxPts == 0 {
		// Score may come from timers/other events; generic rate.
		return int(durSec*25) + 10
	}
	if maxPts < 1 {
		maxPts = 1
	}
	// Stream model: any spawn_entity rule sets the pace.
	streamInterval := 0.0
	for _, r := range doc.Rules {
		for _, a := range r.Actions {
			if a.Type == "spawn_entity" {
				if r.When.Value != nil && *r.When.Value > 0 {
					streamInterval = *r.When.Value
				}
			}
		}
	}
	if streamInterval > 0 {
		return int(math.Ceil(maxPts*1000.0/streamInterval*durSec*1.5)) + int(maxPts)
	}
	// Recycle model over the entry scene's pickups.
	var sc *gameir.Scene
	for i := range doc.Scenes {
		if doc.Scenes[i].ID == doc.Game.EntryScene {
			sc = &doc.Scenes[i]
			break
		}
	}
	if sc == nil && len(doc.Scenes) > 0 {
		sc = &doc.Scenes[0]
	}
	if sc == nil {
		return int(durSec*25) + 10
	}
	n := 0
	vSum := 0.0
	for _, e := range sc.Entities {
		if e.Kind != "pickup" {
			continue
		}
		n++
		v := 50.0
		if e.Physics != nil && len(e.Physics.InitialVelocity) == 2 {
			v = math.Abs(e.Physics.InitialVelocity[1])
		}
		vSum += v
	}
	if n == 0 {
		return int(durSec*25) + 10
	}
	h := 844.0
	if len(sc.WorldSize) == 2 && sc.WorldSize[1] > 0 {
		h = sc.WorldSize[1]
	}
	v := vSum / float64(n)
	g := 0.0
	if len(sc.Gravity) == 2 {
		g = math.Abs(sc.Gravity[1])
	}
	var fall float64
	switch {
	case g > 0:
		fall = (-v + math.Sqrt(v*v+2*g*h)) / g
	case v > 0:
		fall = h / v
	default:
		return int(durSec*25) + 10
	}
	if fall <= 0 {
		return int(durSec*25) + 10
	}
	maxRate := float64(n) / fall // catches per second, perfect play
	return int(math.Ceil(maxRate*maxPts*durSec*1.5)) + int(maxPts)
}

// boardRuns returns the runs on one board (game + IR hash), filtered
// to the window and sorted by the board's metric.
func (s *Server) boardRuns(gameID, hash, window string, doc *gameir.Document) []RunRecord {
	metric, order := boardMetric(doc)
	cutoff := time.Time{}
	if window != "all" {
		cutoff = time.Now().Add(-7 * 24 * time.Hour)
	}
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	var out []RunRecord
	for _, rec := range s.runs {
		if rec.GameID != gameID || rec.IRHash != hash {
			continue
		}
		if !cutoff.IsZero() && rec.At.Before(cutoff) {
			continue
		}
		if metric == "time" && !rec.Won {
			continue // a time board ranks finished runs only
		}
		cp := rec
		cp.Inputs = nil // boards never carry traces
		out = append(out, cp)
	}
	less := func(i, j int) bool {
		a, b := out[i], out[j]
		var ka, kb float64
		switch metric {
		case "time":
			ka, kb = float64(a.DurationMs), float64(b.DurationMs)
		default: // score (and streak, ranked by score in v1)
			ka, kb = float64(a.Score), float64(b.Score)
		}
		if ka != kb {
			if order == "asc" {
				return ka < kb
			}
			return ka > kb
		}
		return a.At.Before(b.At) // earlier run wins a tie
	}
	sort.SliceStable(out, less)
	return out
}

func boardMetric(doc *gameir.Document) (string, string) {
	metric, order := "score", "desc"
	if doc != nil && doc.Experience != nil {
		if doc.Experience.Leaderboard.Metric != "" {
			metric = doc.Experience.Leaderboard.Metric
		}
		if doc.Experience.Leaderboard.Order != "" {
			order = doc.Experience.Leaderboard.Order
		}
	}
	return metric, order
}

// POST /v1/games/{id}/runs — submit a finished run.
func (s *Server) handleSubmitRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	game, ok := s.store.GetGame(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	var req struct {
		Name       string     `json:"name"`
		Score      int        `json:"score"`
		Won        bool       `json:"won"`
		DurationMs int        `json:"durationMs"`
		Seed       uint32     `json:"seed"`
		IRHash     string     `json:"irHash"`
		Inputs     []RunInput `json:"inputs"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Score < 0 {
		writeErr(w, http.StatusBadRequest, "score must be >= 0")
		return
	}
	if req.DurationMs < 1000 || req.DurationMs > 30*60*1000 {
		writeErr(w, http.StatusBadRequest, "durationMs out of range")
		return
	}
	if len(req.Inputs) > 2000 {
		writeErr(w, http.StatusBadRequest, "input trace too large")
		return
	}
	for _, inp := range req.Inputs {
		if inp.Tick < 0 {
			writeErr(w, http.StatusBadRequest, "input tick must be >= 0")
			return
		}
		switch inp.Type {
		case "pointer", "pointerup", "key", "keyup":
		default:
			writeErr(w, http.StatusBadRequest, "unknown input type")
			return
		}
	}
	// The board is the game's CURRENT IR; a run claiming a different
	// hash played a different build and doesn't belong on this board.
	currentHash := irHashOf(game.GameIR)
	hash := req.IRHash
	if hash == "" {
		hash = currentHash
	}
	if hash != currentHash {
		writeErr(w, http.StatusConflict, "run is for a different game version")
		return
	}
	if cap := maxPlausibleScore(game.GameIR, req.DurationMs); req.Score > cap {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": "implausible_score", "cap": cap,
		})
		return
	}
	name, filtered := sanitizeName(req.Name)
	s.runsMu.Lock()
	s.runSeq++
	rec := RunRecord{
		ID:     "run-" + time.Now().Format("20060102") + "-" + itoa(s.runSeq),
		GameID: id, IRHash: hash, Name: name,
		Score: req.Score, Won: req.Won, DurationMs: req.DurationMs,
		Seed: req.Seed, Inputs: req.Inputs, At: time.Now(),
	}
	s.runs = append(s.runs, rec)
	s.runsMu.Unlock()
	s.appendRunFile(rec)
	// Rank on the weekly board (the default view).
	board := s.boardRuns(id, hash, "week", game.GameIR)
	rank := 0
	for i, e := range board {
		if e.ID == rec.ID {
			rank = i + 1
			break
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"runId": rec.ID, "rank": rank, "verified": false, "nameFiltered": filtered,
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// GET /v1/games/{id}/leaderboard?window=week|all — the board for the
// game's current IR version.
func (s *Server) handleLeaderboardV2(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	game, ok := s.store.GetGame(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	window := r.URL.Query().Get("window")
	if window != "all" {
		window = "week"
	}
	hash := irHashOf(game.GameIR)
	board := s.boardRuns(id, hash, window, game.GameIR)
	metric, order := boardMetric(game.GameIR)
	type entry struct {
		Rank       int       `json:"rank"`
		RunID      string    `json:"runId"`
		Name       string    `json:"name"`
		Score      int       `json:"score"`
		Won        bool      `json:"won"`
		DurationMs int       `json:"durationMs"`
		Verified   *bool     `json:"verified,omitempty"`
		At         time.Time `json:"at"`
	}
	entries := []entry{}
	for i, rec := range board {
		if i >= 50 {
			break
		}
		entries = append(entries, entry{
			Rank: i + 1, RunID: rec.ID, Name: rec.Name, Score: rec.Score,
			Won: rec.Won, DurationMs: rec.DurationMs, Verified: rec.Verified, At: rec.At,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"gameId": id, "irHash": hash, "window": window,
		"metric": metric, "order": order, "entries": entries,
	})
}

// GET /v1/runs/{id} — the full run, trace included (the replay fetch).
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	rid := r.PathValue("id")
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	for _, rec := range s.runs {
		if rec.ID == rid {
			writeJSON(w, http.StatusOK, rec)
			return
		}
	}
	writeErr(w, http.StatusNotFound, "run not found")
}

// POST /v1/runs/{id}/verification — a replay's verdict. First
// verdict wins; later reports are acknowledged but do not rewrite
// history.
func (s *Server) handleRunVerification(w http.ResponseWriter, r *http.Request) {
	rid := r.PathValue("id")
	var req struct {
		Match       bool `json:"match"`
		ActualScore int  `json:"actualScore"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s.runsMu.Lock()
	defer s.runsMu.Unlock()
	for i := range s.runs {
		if s.runs[i].ID != rid {
			continue
		}
		if s.runs[i].Verified != nil {
			writeJSON(w, http.StatusOK, map[string]any{"verified": *s.runs[i].Verified, "alreadyRecorded": true})
			return
		}
		m := req.Match
		a := req.ActualScore
		s.runs[i].Verified = &m
		s.runs[i].ActualScore = &a
		writeJSON(w, http.StatusOK, map[string]any{"verified": m})
		return
	}
	writeErr(w, http.StatusNotFound, "run not found")
}
