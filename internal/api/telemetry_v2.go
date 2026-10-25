package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Telemetry v2 (Phase A of the IR 1.2 rollout): the runtime streams
// structured play events — session_start, perf_window (frame-time
// histogram buckets over 5s of visible play), milestone, run_end — to
// POST /v1/telemetry/events. They are filed as JSONL and indexed in
// memory for two consumers: GET /v1/telemetry/perf (per-game quality
// aggregation: jank rate, frame-time p95, win rate, onboarding
// medians) and the rate-based auto-quarantine (error reports divided
// by sessions, so a busy game cannot quarantine itself by volume).

type teleEventV2 struct {
	SchemaVersion  int    `json:"schemaVersion"`
	EventID        string `json:"eventId,omitempty"`
	GameID         string `json:"gameId"`
	RuntimeVersion string `json:"runtimeVersion"`
	SessionID      string `json:"sessionId"`
	Kind           string `json:"kind"`
	Quality        string `json:"quality"`
	DeviceClass    string `json:"deviceClass"`
	At             string `json:"at"`
	// session_start
	Seed      int64  `json:"seed,omitempty"`
	Shell     bool   `json:"shell,omitempty"`
	BootMs    int    `json:"bootMs,omitempty"`
	Renderer  string `json:"renderer,omitempty"`
	IRVersion string `json:"irVersion,omitempty"`
	// perf_window
	Frames         int   `json:"frames,omitempty"`
	Buckets        []int `json:"buckets,omitempty"`
	LongestFrameMs int   `json:"longestFrameMs,omitempty"`
	Entities       int   `json:"entities,omitempty"`
	Particles      int   `json:"particles,omitempty"`
	// milestone
	Milestone string `json:"milestone,omitempty"`
	Ms        int    `json:"ms,omitempty"`
	// run_end
	Won        bool `json:"won,omitempty"`
	Score      int  `json:"score,omitempty"`
	DurationMs int  `json:"durationMs,omitempty"`
	Inputs     int  `json:"inputs,omitempty"`
	DeadInputs int  `json:"deadInputs,omitempty"`
	Retries    int  `json:"retries,omitempty"`
}

type teleSession struct {
	GameID      string
	At          time.Time
	DeviceClass string
	Quality     string
}

type telePerfWin struct {
	GameID  string
	At      time.Time
	Frames  int
	Buckets [6]int
	Longest int
}

type teleRun struct {
	GameID     string
	At         time.Time
	Won        bool
	DurationMs int
	Inputs     int
	DeadInputs int
}

type teleMile struct {
	GameID string
	At     time.Time
	Name   string
	Ms     int
}

const teleV2IndexCap = 20000

var teleV2Kinds = map[string]bool{
	"session_start": true, "perf_window": true, "milestone": true, "run_end": true,
}

// POST /v1/telemetry/events — beacon target for telemetry v2. Accepts
// one event, files it, indexes it, answers 202. Malformed or unknown
// kinds are 400 (the runtime only logs those; nothing retries).
func validTelemetryV2Event(ev teleEventV2) bool {
	if ev.GameID == "" || len(ev.GameID) > 256 || len(ev.SessionID) > 256 ||
		len(ev.RuntimeVersion) > 128 || len(ev.Quality) > 64 || len(ev.DeviceClass) > 128 ||
		len(ev.Renderer) > 128 || len(ev.IRVersion) > 64 || len(ev.Milestone) > 128 {
		return false
	}
	if ev.BootMs < 0 || ev.Frames < 0 || ev.LongestFrameMs < 0 ||
		ev.Entities < 0 || ev.Particles < 0 || ev.Ms < 0 || ev.Score < 0 ||
		ev.DurationMs < 0 || ev.Inputs < 0 || ev.DeadInputs < 0 || ev.Retries < 0 {
		return false
	}
	if ev.DeadInputs > ev.Inputs || len(ev.Buckets) > 6 { return false }
	for _, bucket := range ev.Buckets { if bucket < 0 { return false } }
	return true
}

func (s *Server) handleTelemetryEvent(w http.ResponseWriter, r *http.Request) {
	var ev teleEventV2
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&ev); err != nil {
		http.Error(w, "bad event", http.StatusBadRequest)
		return
	}
	if ev.SchemaVersion == 0 {
		// Older clients omitted the field; interpret those events as the current v2 schema.
		ev.SchemaVersion = 2
	}
	if ev.SchemaVersion != 2 {
		http.Error(w, "unsupported telemetry schemaVersion", http.StatusBadRequest)
		return
	}
	if !teleV2Kinds[ev.Kind] {
		http.Error(w, "unknown telemetry event kind", http.StatusBadRequest)
		return
	}
	if !validTelemetryV2Event(ev) {
		http.Error(w, "invalid telemetry event fields", http.StatusBadRequest)
		return
	}
	if ev.EventID != "" && (ev.SessionID == "" || len(ev.EventID) > 128) {
		http.Error(w, "eventId requires sessionId and must be at most 128 bytes", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	s.teleMu.Lock()
	if ev.EventID != "" {
		if s.teleV2EventIDs == nil {
			s.teleV2EventIDs = map[string]struct{}{}
		}
		key := ev.SessionID + "\x00" + ev.EventID
		if _, exists := s.teleV2EventIDs[key]; exists {
			s.teleMu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			return
		}
	}
	rec := map[string]any{"receivedAt": now.Format(time.RFC3339), "event": ev}
	line, err := json.Marshal(rec)
	if err != nil {
		s.teleMu.Unlock()
		http.Error(w, "could not encode event", http.StatusInternalServerError)
		return
	}
	telemetryMu.Lock()
	mkErr := os.MkdirAll(telemetryDir(), 0o755)
	if mkErr == nil {
		var f *os.File
		f, mkErr = os.OpenFile(filepath.Join(telemetryDir(), "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if mkErr == nil {
			_, mkErr = f.Write(append(line, '\n'))
			closeErr := f.Close()
			if mkErr == nil {
				mkErr = closeErr
			}
		}
	}
	telemetryMu.Unlock()
	if mkErr != nil {
		s.teleMu.Unlock()
		http.Error(w, "could not persist event", http.StatusServiceUnavailable)
		return
	}
	if ev.Kind == "session_start" {
		s.recordSocialPlay(userID(r), ev.GameID)
	}
	if ev.Kind == "run_end" && ev.Won {
		s.recordSocialCompletion(userID(r), ev.GameID, ev.DurationMs)
	}
	switch ev.Kind {
	case "session_start":
		s.teleSessions = append(s.teleSessions, teleSession{GameID: ev.GameID, At: now, DeviceClass: ev.DeviceClass, Quality: ev.Quality})
	case "perf_window":
		pw := telePerfWin{GameID: ev.GameID, At: now, Frames: ev.Frames, Longest: ev.LongestFrameMs}
		for i := 0; i < 6 && i < len(ev.Buckets); i++ {
			pw.Buckets[i] = ev.Buckets[i]
		}
		s.telePerf = append(s.telePerf, pw)
	case "milestone":
		s.teleMiles = append(s.teleMiles, teleMile{GameID: ev.GameID, At: now, Name: ev.Milestone, Ms: ev.Ms})
	case "run_end":
		s.teleRuns = append(s.teleRuns, teleRun{GameID: ev.GameID, At: now, Won: ev.Won, DurationMs: ev.DurationMs, Inputs: ev.Inputs, DeadInputs: ev.DeadInputs})
	}
	if len(s.teleSessions) > teleV2IndexCap {
		s.teleSessions = s.teleSessions[len(s.teleSessions)-teleV2IndexCap:]
	}
	if len(s.telePerf) > teleV2IndexCap {
		s.telePerf = s.telePerf[len(s.telePerf)-teleV2IndexCap:]
	}
	if len(s.teleRuns) > teleV2IndexCap {
		s.teleRuns = s.teleRuns[len(s.teleRuns)-teleV2IndexCap:]
	}
	if len(s.teleMiles) > teleV2IndexCap {
		s.teleMiles = s.teleMiles[len(s.teleMiles)-teleV2IndexCap:]
	}
	if ev.EventID != "" {
		key := ev.SessionID + "\x00" + ev.EventID
		s.teleV2EventIDs[key] = struct{}{}
		s.teleV2EventOrder = append(s.teleV2EventOrder, key)
		if len(s.teleV2EventOrder) > teleV2IndexCap {
			oldest := s.teleV2EventOrder[0]
			s.teleV2EventOrder = s.teleV2EventOrder[1:]
			delete(s.teleV2EventIDs, oldest)
		}
	}
	s.teleMu.Unlock()
	w.WriteHeader(http.StatusAccepted)
}

func medianInt(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]int{}, vals...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}

type perfSummary struct {
	Sessions            int     `json:"sessions"`
	Runs                int     `json:"runs"`
	WinRate             float64 `json:"winRate"`
	AvgRunMs            int     `json:"avgRunMs"`
	DeadInputRate       float64 `json:"deadInputRate"`
	FirstInputMedianMs  int     `json:"firstInputMedianMs"`
	FirstRewardMedianMs int     `json:"firstRewardMedianMs"`
	PerfWindows         int     `json:"perfWindows"`
	Frames              int     `json:"frames"`
	JankRate            float64 `json:"jankRate"`   // share of frames slower than 33ms
	P95FrameMs          int     `json:"p95FrameMs"` // bucket-edge approximation
}

// perfSummaries aggregates the indexed telemetry v2 events into
// per-game quality summaries. Shared by the perf endpoint, the
// experience-incident derivation, and the difficulty tuner.
func (s *Server) perfSummaries() map[string]*perfSummary {
	s.teleMu.Lock()
	games := map[string]*perfSummary{}
	buckets := map[string][6]int{}
	firstIn := map[string][]int{}
	firstRew := map[string][]int{}
	runMs := map[string]int{}
	inputs := map[string]int{}
	get := func(id string) *perfSummary {
		p, ok := games[id]
		if !ok {
			p = &perfSummary{}
			games[id] = p
		}
		return p
	}
	for _, ss := range s.teleSessions {
		get(ss.GameID).Sessions++
	}
	for _, pw := range s.telePerf {
		p := get(pw.GameID)
		p.PerfWindows++
		p.Frames += pw.Frames
		b := buckets[pw.GameID]
		for i := 0; i < 6; i++ {
			b[i] += pw.Buckets[i]
		}
		buckets[pw.GameID] = b
	}
	for _, rn := range s.teleRuns {
		p := get(rn.GameID)
		p.Runs++
		if rn.Won {
			p.WinRate++
		}
		runMs[rn.GameID] += rn.DurationMs
		inputs[rn.GameID] += rn.Inputs
		p.DeadInputRate += float64(rn.DeadInputs)
	}
	for _, m := range s.teleMiles {
		switch m.Name {
		case "first_input":
			firstIn[m.GameID] = append(firstIn[m.GameID], m.Ms)
		case "first_reward":
			firstRew[m.GameID] = append(firstRew[m.GameID], m.Ms)
		}
	}
	s.teleMu.Unlock()
	edges := [6]int{8, 17, 25, 33, 50, 100}
	for id, p := range games {
		if p.Runs > 0 {
			p.WinRate = p.WinRate / float64(p.Runs)
			p.AvgRunMs = runMs[id] / p.Runs
			if inputs[id] > 0 {
				p.DeadInputRate = p.DeadInputRate / float64(inputs[id])
			} else {
				p.DeadInputRate = 0
			}
		}
		p.FirstInputMedianMs = medianInt(firstIn[id])
		p.FirstRewardMedianMs = medianInt(firstRew[id])
		b := buckets[id]
		total := 0
		for i := 0; i < 6; i++ {
			total += b[i]
		}
		if total > 0 {
			p.JankRate = float64(b[4]+b[5]) / float64(total)
			cum := 0
			p.P95FrameMs = edges[5]
			for i := 0; i < 6; i++ {
				cum += b[i]
				if float64(cum) >= 0.95*float64(total) {
					p.P95FrameMs = edges[i]
					break
				}
			}
		}
	}
	return games
}

// GET /v1/telemetry/perf — per-game quality aggregation over the
// indexed telemetry v2 events. ?game=<id> narrows to one game.
func (s *Server) handlePerfSummary(w http.ResponseWriter, r *http.Request) {
	if !requireTelemetryAdminToken(w, r) {
		return
	}
	only := r.URL.Query().Get("game")
	games := s.perfSummaries()
	out := map[string]*perfSummary{}
	for id, p := range games {
		if only == "" || only == id {
			out[id] = p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// sessionsInWindow counts telemetry-v2 session starts for a game in a
// trailing window — the denominator for rate-based quarantine. Caller
// holds s.teleMu.
func (s *Server) sessionsInWindowLocked(gameID string, cutoff time.Time) int {
	n := 0
	for _, ss := range s.teleSessions {
		if ss.GameID == gameID && !ss.At.Before(cutoff) {
			n++
		}
	}
	return n
}
