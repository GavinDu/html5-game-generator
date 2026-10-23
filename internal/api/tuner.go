package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"doodlequest/internal/gameir"
)

// Difficulty tuner (Phase E): shadow-mode pacing proposals derived
// from measured telemetry — the repair loop's difficulty tier.
// Rules of the house, same as repairs: ONE knob per proposal, the
// proposal changes nothing until a human applies it, application
// revalidates the whole IR, and insufficient data is reported as
// insufficient data (never a guess dressed as a measurement).

type tunerProposal struct {
	ID        string  `json:"id"`
	GameID    string  `json:"gameId"`
	Knob      string  `json:"knob"` // ramp_speed | health
	From      float64 `json:"from"`
	To        float64 `json:"to"`
	Direction string  `json:"direction"` // ease | tighten
	Rationale string  `json:"rationale"`
	Status    string  `json:"status"` // proposed | applied | discarded
	CreatedAt string  `json:"createdAt"`
}

const (
	tunerMinRuns    = 20
	tunerEaseWin    = 0.05 // win rate at or below → too hard
	tunerTightenWin = 0.95 // win rate at or above → no challenge
	tunerStep       = 0.10 // one knob step: 10% of the ramp (or ±1 health)
)

// planTuning is the pure decision: given a perf summary and the
// game's experience block, return a proposal (or a reason there is
// none). Kept pure for tests.
func planTuning(gameID string, p *perfSummary, exp *gameir.Experience) (*tunerProposal, string) {
	if exp == nil {
		return nil, "game declares no experience block"
	}
	if p == nil || p.Runs < tunerMinRuns {
		runs := 0
		if p != nil {
			runs = p.Runs
		}
		return nil, fmt.Sprintf("insufficient data: %d runs measured, %d needed", runs, tunerMinRuns)
	}
	rampFinal := 0.0
	for _, pt := range exp.Pacing.Ramp {
		if pt.SpeedMul > rampFinal {
			rampFinal = pt.SpeedMul
		}
	}
	mk := func(knob string, from, to float64, dir, why string) *tunerProposal {
		return &tunerProposal{
			GameID: gameID, Knob: knob, From: from, To: to, Direction: dir,
			Rationale: why, Status: "proposed", CreatedAt: time.Now().Format(time.RFC3339),
		}
	}
	switch {
	case p.WinRate <= tunerEaseWin:
		why := fmt.Sprintf("win rate %.1f%% over %d runs (ease threshold %.0f%%)", 100*p.WinRate, p.Runs, 100*tunerEaseWin)
		if rampFinal > 1.0 {
			to := math.Round((1+(rampFinal-1)*(1-tunerStep))*100) / 100
			return mk("ramp_speed", rampFinal, to, "ease", why+": final ramp speedMul eased 10% toward 1.0"), ""
		}
		if exp.Failure.Mode == "health" && exp.Failure.Health > 0 {
			return mk("health", float64(exp.Failure.Health), float64(exp.Failure.Health+1), "ease", why+": health +1"), ""
		}
		return nil, "game is too hard but exposes no tunable knob (no ramp above 1.0, no health pool)"
	case p.WinRate >= tunerTightenWin:
		why := fmt.Sprintf("win rate %.1f%% over %d runs (tighten threshold %.0f%%)", 100*p.WinRate, p.Runs, 100*tunerTightenWin)
		if rampFinal > 0 {
			to := math.Round((1+(rampFinal-1)*(1+tunerStep))*100) / 100
			if to > 2.0 {
				to = 2.0
			}
			if to != rampFinal {
				return mk("ramp_speed", rampFinal, to, "tighten", why+": final ramp speedMul raised 10%"), ""
			}
		}
		if exp.Failure.Mode == "health" && exp.Failure.Health > 1 {
			return mk("health", float64(exp.Failure.Health), float64(exp.Failure.Health-1), "tighten", why+": health −1"), ""
		}
		return nil, "game is trivially easy but exposes no tunable knob"
	}
	return nil, fmt.Sprintf("win rate %.1f%% over %d runs is inside the healthy band — no tuning proposed", 100*p.WinRate, p.Runs)
}

// applyTuning mutates a copy of the IR per the proposal's knob.
func applyTuning(doc *gameir.Document, prop *tunerProposal) error {
	exp := doc.Experience
	if exp == nil {
		return fmt.Errorf("no experience block")
	}
	switch prop.Knob {
	case "ramp_speed":
		// Scale the peak point(s): every ramp point at the old maximum
		// moves to the proposed value; intermediate points scale
		// proportionally so the ramp's shape survives.
		if prop.From <= 0 {
			return fmt.Errorf("invalid ramp origin")
		}
		for i, pt := range exp.Pacing.Ramp {
			if pt.SpeedMul == prop.From {
				exp.Pacing.Ramp[i].SpeedMul = prop.To
			} else if pt.SpeedMul > 1.0 {
				exp.Pacing.Ramp[i].SpeedMul = math.Round((1+(pt.SpeedMul-1)*(prop.To-1)/(prop.From-1))*100) / 100
			}
		}
	case "health":
		exp.Failure.Health = int(prop.To)
	default:
		return fmt.Errorf("unknown knob %q", prop.Knob)
	}
	return nil
}

func (s *Server) tunerPropose(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	var exp *gameir.Experience
	if game.GameIR != nil {
		exp = game.GameIR.Experience
	}
	summary := s.perfSummaries()[game.ID]
	prop, reason := planTuning(game.ID, summary, exp)
	if prop == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "no_proposal", "reason": reason})
		return
	}
	s.tunerMu.Lock()
	s.tunerSeq++
	prop.ID = fmt.Sprintf("tune-%d", s.tunerSeq)
	if s.tunerProposals == nil {
		s.tunerProposals = map[string]*tunerProposal{}
	}
	s.tunerProposals[prop.ID] = prop
	s.tunerMu.Unlock()
	writeJSON(w, http.StatusCreated, prop)
}

func (s *Server) tunerList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.tunerMu.Lock()
	out := []*tunerProposal{}
	for _, p := range s.tunerProposals {
		if p.GameID == id {
			out = append(out, p)
		}
	}
	s.tunerMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"proposals": out})
}

func (s *Server) tunerResolve(w http.ResponseWriter, r *http.Request, apply bool) {
	if apply && !requireModerationToken(w, r) {
		return
	}
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	s.tunerMu.Lock()
	prop := s.tunerProposals[r.PathValue("pid")]
	s.tunerMu.Unlock()
	if prop == nil || prop.GameID != game.ID || prop.Status != "proposed" {
		writeErr(w, http.StatusConflict, "no live tuner proposal with that id")
		return
	}
	if !apply {
		s.tunerMu.Lock()
		prop.Status = "discarded"
		s.tunerMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"discarded": true, "id": prop.ID})
		return
	}
	if game.GameIR == nil || game.GameIR.Experience == nil {
		writeErr(w, http.StatusConflict, "game has no experience block to tune")
		return
	}
	raw, err := json.Marshal(game.GameIR)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "marshal IR: "+err.Error())
		return
	}
	doc, err := gameir.DecodeAndValidate(raw)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "decode IR: "+err.Error())
		return
	}
	if err := applyTuning(doc, prop); err != nil {
		writeErr(w, http.StatusConflict, "apply tuning: "+err.Error())
		return
	}
	if err := gameir.Validate(doc); err != nil {
		writeErr(w, http.StatusConflict, "tuned IR does not validate: "+err.Error())
		return
	}
	game.GameIR = doc
	game.UpdatedAt = time.Now()
	s.store.UpdateGame(game)
	s.tunerMu.Lock()
	prop.Status = "applied"
	s.tunerMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"applied": true, "id": prop.ID, "knob": prop.Knob, "from": prop.From, "to": prop.To})
}
