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
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"doodlequest/internal/gameir"
)

// irSHA256Of identifies the complete canonical IR content. Unlike the
// historical 12-hex leaderboard key, this is suitable for strict version
// attribution in a playtest/publishing evidence contract.
func irSHA256Of(doc *gameir.Document) string {
	if doc == nil {
		return ""
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// irHashOf retains the original short leaderboard/run key for compatibility.
func irHashOf(doc *gameir.Document) string {
	digest := irSHA256Of(doc)
	if digest == "" {
		return ""
	}
	return digest[:12]
}

// mintRunID is server-owned identity, separate from the deterministic
// seed/tick replay. The previous same-day process-local counter restarted
// at 1 after loading runs.jsonl, reusing persisted IDs and making
// GET /v1/runs/{id} resolve an unrelated earlier attempt.
// 128 bits of CSPRNG entropy remain unique across restarts and independent
// API instances without client-supplied data.
func mintRunID(now time.Time) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "run-" + now.Format("20060102") + "-" + hex.EncodeToString(nonce[:]), nil
}

type RunInput struct {
	Tick   int    `json:"tick"`
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

// Matches String.length in the trusted JavaScript replay recorder. Go strings
// are UTF-8, but JS counts UTF-16 code units (two for supplementary scalars).
// Fail early at the bound without allocating a temporary rune/UTF-16 slice.
const maxReplayInputDetailUTF16Units = 128

func validReplayInputDetail(detail string) bool {
	units := 0
	for _, codePoint := range detail {
		units++
		if codePoint > 0xFFFF {
			units++
		}
		if units > maxReplayInputDetailUTF16Units {
			return false
		}
	}
	return true
}

// A stored pointer event must have exactly the two canonical integer
// coordinates emitted by the preinstalled Pixi input adapter. Number()
// in the historical replay path would silently skip invalid coordinates
// or accept ambiguous decimal/exponent/trailing-component representations.
// One million logical pixels accommodates off-canvas pointer capture while
// preventing unbounded values from reaching a future replay executor.
const maxReplayPointerCoordinate int64 = 1_000_000

func validReplayPointerDetail(detail string) bool {
	coords := strings.Split(detail, ",")
	if len(coords) != 2 {
		return false
	}
	for _, coord := range coords {
		value, err := strconv.ParseInt(coord, 10, 64)
		if err != nil || strconv.FormatInt(value, 10) != coord ||
			value < -maxReplayPointerCoordinate || value > maxReplayPointerCoordinate {
			return false
		}
	}
	return true
}

// RunRecord stores a full server-owned canonical IR digest for *new* runs.
// The old 12-hex board key is insufficient for future replay provenance.
// Legacy records omit IRSHA256; never infer an unrecorded historical digest.
// canonicalRunInputSHA256 fingerprints admitted replay *evidence* using a
// server-owned, fixed-field serialization. Inputs retain their tick/order:
// multiple actions on the same tick are not commutative. This is not a
// simulation, score verifier, signature, or proof of client execution.
func canonicalRunInputSHA256(gameID, fullIRSHA256 string, seed uint32, durationMs int, inputs []RunInput) (string, error) {
	type replayInputEvidenceV1 struct {
		Schema     string     `json:"schema"`
		GameID     string     `json:"gameId"`
		IRSHA256   string     `json:"irSha256"`
		Seed       uint32     `json:"seed"`
		DurationMs int        `json:"durationMs"`
		Inputs     []RunInput `json:"inputs"`
	}
	if gameID == "" || len(fullIRSHA256) != 64 || durationMs < 0 {
		return "", fmt.Errorf("invalid replay evidence identity")
	}
	if inputs == nil {
		inputs = []RunInput{}
	}
	raw, err := json.Marshal(replayInputEvidenceV1{
		Schema: "replay-input-evidence-v1", GameID: gameID, IRSHA256: fullIRSHA256,
		Seed: seed, DurationMs: durationMs, Inputs: inputs,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

type RunRecord struct {
	ID         string     `json:"id"`
	GameID     string     `json:"gameId"`
	IRHash     string     `json:"irHash"`
	IRSHA256   string     `json:"irSha256,omitempty"`
	Name       string     `json:"name"`
	Score      int        `json:"score"`
	Won        bool       `json:"won"`
	DurationMs int        `json:"durationMs"`
	Seed       uint32     `json:"seed"`
	Inputs     []RunInput `json:"inputs,omitempty"`
	At         time.Time  `json:"at"`
	// Server-computed canonical admitted seed/tick-input evidence. This
	// does not attest that the browser actually executed those inputs.
	ReplayInputSHA256 string `json:"replayInputSha256,omitempty"`
	// Legacy Verified/ActualScore were once client-supplied. Retain them
	// only for controlled audit; never expose them as trusted attestation
	// until a server-owned replay proof and migration exist.
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

// appendRunFile is the durable admission boundary for a replay trace.
// The caller holds runsMu: persistence and in-memory publication must be in
// the same order, and an I/O failure must never yield a successful runId.
// Existing JSONL logs remain readable; this is a single-process guarantee,
// not a replacement for transactional multi-writer storage.
func (s *Server) appendRunFile(rec RunRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(telemetryDir(), 0o755); err != nil {
		return err
	}
	// Replay traces can contain private input timing, so newly-created logs
	// must not be world-readable. Existing file permissions are unchanged.
	f, err := os.OpenFile(filepath.Join(telemetryDir(), "runs.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
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
	// The trusted renderer records inputs against the 60 Hz fixed-step
	// game clock and reports duration from that same clock. Reject events
	// far beyond a claimed run: they cannot participate in the submitted
	// outcome and must not become work for a future server replay executor.
	// One second of clock/rounding slack preserves old legitimate traces;
	// this is a resource bound, NOT independent score attestation.
	const replayStepsPerSecond = 60
	const legacyReplaySlackSeconds = 1
	maxRecordedTick := req.DurationMs*replayStepsPerSecond/1000 + replayStepsPerSecond*legacyReplaySlackSeconds
	// Authoritative replay events are ordered by the fixed-step simulation
	// tick. Multiple actions on one tick are valid and retain their original
	// sequence, but a backwards tick cannot be replayed canonically.
	previousTick := -1
	for _, inp := range req.Inputs {
		if inp.Tick < 0 {
			writeErr(w, http.StatusBadRequest, "input tick must be >= 0")
			return
		}
		if inp.Tick > maxRecordedTick {
			writeErr(w, http.StatusBadRequest, "input tick exceeds run duration")
			return
		}
		if inp.Tick < previousTick {
			writeErr(w, http.StatusBadRequest, "input ticks must be nondecreasing")
			return
		}
		previousTick = inp.Tick
		// Admission must match the 128 UTF-16-code-unit limit of the
		// trusted browser recorder; HTTP callers cannot bypass that budget.
		if !validReplayInputDetail(inp.Detail) {
			writeErr(w, http.StatusBadRequest, "input detail exceeds replay budget")
			return
		}
		switch inp.Type {
		case "pointer":
			if !validReplayPointerDetail(inp.Detail) {
				writeErr(w, http.StatusBadRequest, "invalid pointer replay coordinates")
				return
			}
		case "pointerup":
			if inp.Detail != "" {
				writeErr(w, http.StatusBadRequest, "pointer release must have no detail")
				return
			}
		case "key", "keyup":
			// Preserve the version 1.x input-binding vocabulary while its
			// independent versioned replay grammar is being designed.
		default:
			writeErr(w, http.StatusBadRequest, "unknown input type")
			return
		}
	}
	// The board is the game's CURRENT IR; a run claiming a different
	// hash played a different build and doesn't belong on this board.
	fullIRSHA256 := irSHA256Of(game.GameIR)
	if len(fullIRSHA256) != 64 {
		writeErr(w, http.StatusServiceUnavailable, "unable to identify game version")
		return
	}
	currentHash := fullIRSHA256[:12]
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
	// Compute replay evidence identity before a run ID or leaderboard record
	// exists. A client never supplies this digest as an authority.
	replayInputSHA256, digestErr := canonicalRunInputSHA256(id, fullIRSHA256, req.Seed, req.DurationMs, req.Inputs)
	if digestErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "unable to identify replay inputs")
		return
	}
	name, filtered := sanitizeName(req.Name)
	now := time.Now()
	runID, err := mintRunID(now)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unable to allocate run ID")
		return
	}
	s.runsMu.Lock()
	// Defense in depth against any accidental duplicate, including legacy
	// persisted IDs. An ID must never resolve ambiguously in this process.
	for _, previous := range s.runs {
		if previous.ID == runID {
			s.runsMu.Unlock()
			writeErr(w, http.StatusServiceUnavailable, "unable to allocate unique run ID")
			return
		}
	}
	rec := RunRecord{
		ID:     runID,
		GameID: id, IRHash: hash, IRSHA256: fullIRSHA256,
		ReplayInputSHA256: replayInputSHA256, Name: name,
		Score: req.Score, Won: req.Won, DurationMs: req.DurationMs,
		Seed: req.Seed, Inputs: req.Inputs, At: now,
	}
	// Never publish a run (or its leaderboard rank) until the on-disk
	// replay trace has been written and synced. A failure can be retried
	// without creating an in-memory ghost run that vanishes after restart.
	if err := s.appendRunFile(rec); err != nil {
		s.runsMu.Unlock()
		writeErr(w, http.StatusServiceUnavailable, "unable to persist run")
		return
	}
	s.runs = append(s.runs, rec)
	s.runsMu.Unlock()
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
			// Legacy Verified was writable by the browser and is not proof.
			// Suppress it in public API responses until an independently
			// attested, versioned server result can be represented.
			Won: rec.Won, DurationMs: rec.DurationMs, Verified: nil, At: rec.At,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"gameId": id, "irHash": hash, "window": window,
		"metric": metric, "order": order, "entries": entries,
	})
}

// GET /v1/runs/{id} — a full input trace is sensitive gameplay data.
// Authorize it using the SAME game-delivery policy as the package before
// returning the seed/input history. Never let intermediaries cache traces.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Authorization, X-User-ID")
	rid := r.PathValue("id")
	var found *RunRecord
	s.runsMu.Lock()
	for _, rec := range s.runs {
		if rec.ID == rid {
			copy := rec
			found = &copy
			break
		}
	}
	s.runsMu.Unlock()
	if found == nil {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	game, ok := s.store.GetGame(found.GameID)
	if !ok || !s.canDeliver(game, userID(r)) {
		// Do not reveal the existence of a private, moderated, removed or
		// blocked creator's recording to an unauthorized viewer.
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	// Do not leak historical client-origin "verified" verdicts through
	// recorded-run fetches: the same endpoint is used by the iOS host to
	// determine whether it may present an independently verified result.
	// Keep the original values in storage for explicit audit/remediation.
	found.Verified = nil
	found.ActualScore = nil
	writeJSON(w, http.StatusOK, found)
}

// POST /v1/runs/{id}/verification — legacy endpoint intentionally disabled.
//
// The old handler trusted a browser-supplied {"match":true} boolean and
// marked a run Verified without replaying the seed/inputs on the server.
// An authenticated game player can modify their HTTP request, so neither
// a replay-mode browser session nor a client-held token can attest a score.
//
// Until a server-owned deterministic replay verifier exists, reject every
// client verdict, including negative/unknown-run reports. Do not mutate the
// stored verification state or disclose whether a run ID exists.
func (s *Server) handleRunVerification(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeErr(w, http.StatusForbidden, "server-attested replay verification required")
}
