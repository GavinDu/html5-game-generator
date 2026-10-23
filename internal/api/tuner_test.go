package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

// seedTunerGame loads the real Bobo IR into a memory store and feeds
// the server synthetic telemetry with the given run outcomes.
func seedTunerGame(t *testing.T, srv *Server, runs int, wins int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "inventory", "13ui1k", "game-ir.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := gameir.DecodeAndValidate(raw)
	if err != nil {
		t.Fatal(err)
	}
	g := &model.Game{ID: "sample-rainbow-bear", Title: "Bobo's Rainbow", GameIR: doc, Status: model.GameReady}
	if s, ok := srv.store.(interface{ CreateGame(*model.Game) }); ok {
		s.CreateGame(g)
	} else {
		t.Skip("memory store has no CreateGame")
	}
	for i := 0; i < runs; i++ {
		srv.teleRuns = append(srv.teleRuns, teleRun{
			GameID: g.ID, At: time.Now(), Won: i < wins, DurationMs: 15000, Inputs: 40, DeadInputs: 1,
		})
		srv.teleSessions = append(srv.teleSessions, teleSession{GameID: g.ID, At: time.Now()})
	}
}

func TestTunerInsufficientDataIsHonest(t *testing.T) {
	srv := newTestServer()
	seedTunerGame(t, srv, 5, 0)
	rec := do(t, srv, "POST", "/v1/games/sample-rainbow-bear/tuner/propose", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("propose: %d %s", rec.Code, rec.Body)
	}
	var out map[string]any
	decode(t, rec, &out)
	if out["status"] != "no_proposal" {
		t.Fatalf("expected no_proposal with 5 runs, got %v", out)
	}
}

func TestTunerEaseProposalAppliesThroughValidation(t *testing.T) {
	opEnv(t)
	srv := newTestServer()
	seedTunerGame(t, srv, 30, 0) // 0% win rate over 30 runs
	rec := do(t, srv, "POST", "/v1/games/sample-rainbow-bear/tuner/propose", nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("propose: %d %s", rec.Code, rec.Body)
	}
	var prop tunerProposal
	decode(t, rec, &prop)
	// Bobo's ramp peaks at 1.45 → eased 10% toward 1.0 = 1.41.
	if prop.Knob != "ramp_speed" || prop.Direction != "ease" || prop.To != 1.41 {
		t.Fatalf("unexpected proposal: %+v", prop)
	}
	// Shadow posture: the stored game is untouched until apply.
	game, _ := srv.store.GetGame("sample-rainbow-bear")
	for _, pt := range game.GameIR.Experience.Pacing.Ramp {
		if pt.SpeedMul == 1.41 {
			t.Fatal("proposal mutated the game before apply")
		}
	}
	rec = postOp(t, srv, "/v1/games/sample-rainbow-bear/tuner/proposals/"+prop.ID+"/apply")
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	game, _ = srv.store.GetGame("sample-rainbow-bear")
	peak := 0.0
	for _, pt := range game.GameIR.Experience.Pacing.Ramp {
		if pt.SpeedMul > peak {
			peak = pt.SpeedMul
		}
	}
	if peak != 1.41 {
		t.Fatalf("after apply, ramp peak = %v, want 1.41", peak)
	}
	// Second apply of the same proposal must conflict.
	rec = postOp(t, srv, "/v1/games/sample-rainbow-bear/tuner/proposals/"+prop.ID+"/apply")
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-apply: %d, want 409", rec.Code)
	}
}

func TestPlanTuningHealthyBandStaysQuiet(t *testing.T) {
	exp := &gameir.Experience{}
	exp.Pacing.Ramp = []gameir.ExpRampStep{{At: 0, SpeedMul: 1}, {At: 1, SpeedMul: 1.4}}
	prop, reason := planTuning("g", &perfSummary{Runs: 40, WinRate: 0.5}, exp)
	if prop != nil || reason == "" {
		t.Fatalf("healthy band: prop=%v reason=%q", prop, reason)
	}
}

func TestExperienceIncidentsDeriveFromAggregates(t *testing.T) {
	p := &perfSummary{Sessions: 30, Runs: 30, WinRate: 0.0, DeadInputRate: 0.4, Frames: 10000, JankRate: 0.3, P95FrameMs: 50}
	incs := deriveExperienceIncidents("g", p, 5)
	kinds := map[string]bool{}
	for _, i := range incs {
		kinds[i.Kind] = true
	}
	for _, want := range []string{"experience.jank", "experience.dead_inputs", "experience.no_wins"} {
		if !kinds[want] {
			t.Errorf("missing incident %s (got %v)", want, kinds)
		}
	}
	// Below the session floor, rate claims stay silent.
	p2 := &perfSummary{Sessions: 3, Runs: 3, WinRate: 0, JankRate: 0.9, Frames: 100}
	if got := deriveExperienceIncidents("g", p2, 5); len(got) != 0 {
		t.Errorf("below floor: got %d incidents, want 0", len(got))
	}
}
