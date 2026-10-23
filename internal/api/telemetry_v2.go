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
func (s *Server) handleTelemetryEvent(w http.ResponseWriter, r *http.Request) {
	var ev teleEventV2
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := dec.Decode(&ev); err != nil {
		http.Error(w, "bad event", http.StatusBadRequest)
		return
	}
	if !teleV2Kinds[ev.Kind] || ev.GameID == "" {
		http.Error(w, "unknown kind or missing gameId", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	rec := map[string]any{"receivedAt": now.Format(time.RFC3339), "event": ev}
	if line, err := json.Marshal(rec); err == nil {
		telemetryMu.Lock()
		if mkErr := os.MkdirAll(telemetryDir(), 0o755); mkErr == nil {
			if f, ferr := os.OpenFile(filepath.Join(telemetryDir(), "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); ferr == nil {
				_, _ = f.Write(append(line, '\n'))
				_ = f.Close()
			}
		}
		telemetryMu.Unlock()
	}
	s.teleMu.Lock()
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
