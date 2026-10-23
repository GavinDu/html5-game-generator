package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Runtime-error telemetry (auto detect-and-fix flow). The player page is
// bridgeless in the iOS WebView, so it reports its own uncaught errors via
// navigator.sendBeacon. Reports land as JSONL, grouped by a stable
// fingerprint: the same failure on many sessions — or on 2+ games, which
// marks it a runtime bug rather than a content bug — is one incident.

type runtimeErrorReport struct {
	GameID         string   `json:"gameId"`
	RuntimeVersion string   `json:"runtimeVersion"`
	UserAgent      string   `json:"userAgent"`
	Kind           string   `json:"kind"`
	Name           string   `json:"name"`
	Message        string   `json:"message"`
	Stack          string   `json:"stack"`
	At             string   `json:"at"`
	LogTail        []string `json:"logTail"`
}

var (
	telemetryMu  sync.Mutex
	digitsRe     = regexp.MustCompile(`\d+`)
	hexIDRe      = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)
	whitespaceRe = regexp.MustCompile(`\s+`)
)

// fingerprintRuntimeError hashes the error's identity with volatile detail
// (numbers, hex ids) stripped, plus the runtime version: the same bug on
// the same build maps to one fingerprint wherever it happens.
func fingerprintRuntimeError(rep *runtimeErrorReport) string {
	msg := hexIDRe.ReplaceAllString(strings.ToLower(rep.Message), "#")
	msg = digitsRe.ReplaceAllString(msg, "#")
	msg = whitespaceRe.ReplaceAllString(strings.TrimSpace(msg), " ")
	frames := []string{}
	for _, line := range strings.Split(rep.Stack, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Keep the tail of each frame (source file + position); drop the
		// URL scheme/host so the same build fingerprints identically
		// wherever it is hosted.
		if i := strings.LastIndex(line, "/"); i >= 0 {
			line = line[i+1:]
		}
		frames = append(frames, line)
		if len(frames) == 3 {
			break
		}
	}
	h := sha256.Sum256([]byte(strings.Join([]string{rep.Name, msg, strings.Join(frames, "|"), rep.RuntimeVersion}, "\n")))
	return hex.EncodeToString(h[:])[:16]
}

// POST /v1/telemetry/runtime-error — beacon target; accepts, fingerprints,
// appends one JSONL record, indexes the event for incident aggregation
// and the auto-quarantine thresholds (Phase 2), and answers 202.
func (s *Server) handleRuntimeError(w http.ResponseWriter, r *http.Request) {
	var rep runtimeErrorReport
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&rep); err != nil {
		http.Error(w, "bad report", http.StatusBadRequest)
		return
	}
	fp := fingerprintRuntimeError(&rep)
	rec := map[string]any{
		"receivedAt":  time.Now().UTC().Format(time.RFC3339),
		"fingerprint": fp,
		"report":      rep,
	}
	if line, err := json.Marshal(rec); err == nil {
		telemetryMu.Lock()
		dir := os.Getenv("DQ_TELEMETRY_DIR")
		if dir == "" {
			dir = "telemetry"
		}
		if mkErr := os.MkdirAll(dir, 0o755); mkErr == nil {
			if f, ferr := os.OpenFile(filepath.Join(dir, "runtime-errors.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
		telemetryMu.Unlock()
	}
	// Perf samples are filed (the JSONL above) but never indexed: they
	// are high-volume by design and must not count toward the incident
	// or auto-quarantine thresholds, which are error signals.
	if rep.Kind != "perf" {
		s.observeTelemetry(teleEvent{
			Fingerprint: fp, GameID: rep.GameID, Kind: rep.Kind,
			Name: rep.Name, Message: rep.Message, At: time.Now().UTC(),
		})
	}
	w.WriteHeader(http.StatusAccepted)
}

// --- Phase 2: incident aggregation + auto-quarantine ---------------------

type teleEvent struct {
	Fingerprint string
	GameID      string
	Kind        string
	Name        string
	Message     string
	At          time.Time
}

// Auto-quarantine thresholds, evaluated over a trailing window. Seed
// values from the fused design: one fingerprint repeating is a stronger
// signal than raw volume, so its bar is lower.
const (
	quarantineWindow             = time.Hour
	quarantineGameTotalThreshold = 10 // reports for one game inside the window
	quarantineFingerprintThresh  = 5  // reports for one (game, fingerprint)
	teleIndexCap                 = 10000
)

func telemetryDir() string {
	if d := os.Getenv("DQ_TELEMETRY_DIR"); d != "" {
		return d
	}
	return "telemetry"
}

// loadTeleIndexLocked reads the JSONL backlog once so incidents survive
// restarts of the aggregation view (the raw log remains the source of
// truth). Caller holds s.teleMu.
func (s *Server) loadTeleIndexLocked() {
	if s.teleLoaded {
		return
	}
	s.teleLoaded = true
	raw, err := os.ReadFile(filepath.Join(telemetryDir(), "runtime-errors.jsonl"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			ReceivedAt  string             `json:"receivedAt"`
			Fingerprint string             `json:"fingerprint"`
			Report      runtimeErrorReport `json:"report"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil || rec.Fingerprint == "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339, rec.ReceivedAt)
		s.teleEvents = append(s.teleEvents, teleEvent{
			Fingerprint: rec.Fingerprint, GameID: rec.Report.GameID,
			Kind: rec.Report.Kind, Name: rec.Report.Name,
			Message: rec.Report.Message, At: at,
		})
	}
}

// Rate-based quarantine (Phase A amendment to the Phase-2 counts):
// raw counts punish popular games — a title with 10,000 sessions will
// cross "10 reports in an hour" by being played. The rules now are:
//   - one fingerprint repeating >= 5x for a game always quarantines
//     (a single crash type is a hard signal at any volume);
//   - otherwise the game's error reports must be a RATE: >= 10 reports
//     AND >= 25% of the sessions started in the window, with a
//     20-session floor so tiny samples can't trigger it;
//   - when no session_start events exist for the game (older clients),
//     fall back to the original count rule (>= 10 reports).
const (
	quarantineMinSessions = 20
	quarantineErrorRate   = 0.25
)

// observeTelemetry indexes one accepted report and evaluates the
// auto-quarantine thresholds for its game.
func (s *Server) observeTelemetry(ev teleEvent) {
	s.teleMu.Lock()
	s.loadTeleIndexLocked()
	s.teleEvents = append(s.teleEvents, ev)
	if len(s.teleEvents) > teleIndexCap {
		s.teleEvents = s.teleEvents[len(s.teleEvents)-teleIndexCap:]
	}
	cutoff := ev.At.Add(-quarantineWindow)
	total, sameFP := 0, 0
	for _, e := range s.teleEvents {
		if e.GameID != ev.GameID || e.At.Before(cutoff) {
			continue
		}
		total++
		if e.Fingerprint == ev.Fingerprint {
			sameFP++
		}
	}
	sessions := s.sessionsInWindowLocked(ev.GameID, cutoff)
	s.teleMu.Unlock()
	if ev.GameID == "" {
		return
	}
	trigger := sameFP >= quarantineFingerprintThresh
	reason := ""
	switch {
	case trigger:
		reason = fmt.Sprintf("auto: fingerprint %s repeated %dx in 60m (%d reports, %d sessions)",
			ev.Fingerprint, sameFP, total, sessions)
	case sessions == 0 && total >= quarantineGameTotalThreshold:
		trigger = true
		reason = fmt.Sprintf("auto: %d runtime-error reports in 60m (no session data; count rule), top fingerprint %s x%d",
			total, ev.Fingerprint, sameFP)
	case sessions >= quarantineMinSessions && total >= quarantineGameTotalThreshold &&
		float64(total)/float64(sessions) >= quarantineErrorRate:
		trigger = true
		reason = fmt.Sprintf("auto: error rate %.0f%% — %d reports over %d sessions in 60m, top fingerprint %s x%d",
			100*float64(total)/float64(sessions), total, sessions, ev.Fingerprint, sameFP)
	}
	if !trigger {
		return
	}
	game, ok := s.store.GetGame(ev.GameID)
	if !ok || game.Quarantined {
		return
	}
	game.Quarantined = true
	game.QuarantineReason = reason
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
}

type incident struct {
	Fingerprint   string   `json:"fingerprint"`
	Count         int      `json:"count"`
	Games         []string `json:"games"`
	Kinds         []string `json:"kinds,omitempty"`
	SampleName    string   `json:"sampleName"`
	SampleMessage string   `json:"sampleMessage"`
	FirstSeen     string   `json:"firstSeen"`
	LastSeen      string   `json:"lastSeen"`
	RuntimeScope  bool     `json:"runtimeScope"` // same fingerprint on 2+ games => runtime bug, not content
}

// GET /v1/telemetry/incidents — aggregated incidents over every indexed
// report, worst first.
func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	s.teleMu.Lock()
	s.loadTeleIndexLocked()
	byFP := map[string]*incident{}
	gameSets := map[string]map[string]bool{}
	kindSets := map[string]map[string]bool{}
	for _, e := range s.teleEvents {
		inc, ok := byFP[e.Fingerprint]
		if !ok {
			inc = &incident{Fingerprint: e.Fingerprint, SampleName: e.Name,
				SampleMessage: e.Message, FirstSeen: e.At.Format(time.RFC3339)}
			byFP[e.Fingerprint] = inc
			gameSets[e.Fingerprint] = map[string]bool{}
			kindSets[e.Fingerprint] = map[string]bool{}
		}
		inc.Count++
		if e.At.Format(time.RFC3339) > inc.LastSeen {
			inc.LastSeen = e.At.Format(time.RFC3339)
		}
		if e.At.Format(time.RFC3339) < inc.FirstSeen {
			inc.FirstSeen = e.At.Format(time.RFC3339)
		}
		gameSets[e.Fingerprint][e.GameID] = true
		if e.Kind != "" {
			kindSets[e.Fingerprint][e.Kind] = true
		}
	}
	s.teleMu.Unlock()
	out := []*incident{}
	for fp, inc := range byFP {
		for g := range gameSets[fp] {
			inc.Games = append(inc.Games, g)
		}
		for k := range kindSets[fp] {
			inc.Kinds = append(inc.Kinds, k)
		}
		inc.RuntimeScope = len(inc.Games) >= 2
		out = append(out, inc)
	}
	// Worst first; fingerprint order as the stable tiebreak.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Fingerprint < out[j].Fingerprint
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"incidents":  out,
		"experience": s.allExperienceIncidents(),
	})
}

// POST /v1/games/{id}/quarantine — hide a game from the feed manually.
func (s *Server) handleQuarantine(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req)
	game.Quarantined = true
	game.QuarantineReason = req.Reason
	if game.QuarantineReason == "" {
		game.QuarantineReason = "manual quarantine"
	}
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
	writeJSON(w, http.StatusOK, map[string]any{"game_id": game.ID, "quarantined": true, "reason": game.QuarantineReason})
}

// POST /v1/games/{id}/unquarantine — return a game to the feed and clear
// its window of telemetry events so the same burst cannot instantly
// re-quarantine it; new reports start a fresh window.
func (s *Server) handleUnquarantine(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}
	game.Quarantined = false
	game.QuarantineReason = ""
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
	s.teleMu.Lock()
	kept := s.teleEvents[:0]
	for _, e := range s.teleEvents {
		if e.GameID != game.ID {
			kept = append(kept, e)
		}
	}
	s.teleEvents = kept
	s.teleMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"game_id": game.ID, "quarantined": false})
}
