package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func seedReadyGame(st store.Store, id string) *model.Game {
	g := &model.Game{
		ID: id, Title: "T-" + id, OwnerID: "parent-1",
		Visibility: model.VisibilityPublic, Status: model.GameReady,
		GameIR: &gameir.Document{
			IRVersion: "1.1",
			Game:      gameir.Game{ID: id, Title: "T-" + id, EntryScene: "s1", Orientation: "portrait"},
			Scenes: []gameir.Scene{{ID: "s1", Entities: []gameir.Entity{
				{ID: "player", Kind: "player", Position: []float64{10, 10}},
			}}},
		},
	}
	st.CreateGame(g)
	return g
}

func post(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func feedIDs(t *testing.T, srv *Server) map[string]bool {
	t.Helper()
	rec := get(t, srv, "/v1/feed")
	var body struct {
		Games []*model.Game `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("feed json: %v", err)
	}
	out := map[string]bool{}
	for _, g := range body.Games {
		out[g.ID] = true
	}
	return out
}

const errReport = `{"gameId":"g-crash","runtimeVersion":"2026.10.22.1","kind":"uncaught","name":"TypeError","message":"boom 123","stack":"at x (https://h/web/pixi-runtime.js:1:1)","logTail":[]}`

// Five reports with one fingerprint auto-quarantine the game and hide it
// from the feed; unquarantine restores it.
func TestAutoQuarantineAndRestore(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "g-crash")
	seedReadyGame(st, "g-fine")
	if !feedIDs(t, srv)["g-crash"] {
		t.Fatal("precondition: g-crash should start in the feed")
	}
	for i := 0; i < quarantineFingerprintThresh; i++ {
		rec := post(t, srv, "/v1/telemetry/runtime-error", errReport)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("report %d: want 202, got %d", i, rec.Code)
		}
	}
	game, _ := st.GetGame("g-crash")
	if !game.Quarantined {
		t.Fatal("game should be auto-quarantined at the fingerprint threshold")
	}
	ids := feedIDs(t, srv)
	if ids["g-crash"] {
		t.Error("quarantined game must leave the feed")
	}
	if !ids["g-fine"] {
		t.Error("unrelated game must stay in the feed")
	}
	// Incidents aggregate the burst under one fingerprint.
	rec := get(t, srv, "/v1/telemetry/incidents")
	var inc struct {
		Incidents []incident `json:"incidents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &inc); err != nil {
		t.Fatalf("incidents json: %v", err)
	}
	if len(inc.Incidents) != 1 || inc.Incidents[0].Count != quarantineFingerprintThresh {
		t.Fatalf("incidents: %+v", inc.Incidents)
	}
	// Unquarantine restores the feed and clears the burst window.
	if rec := post(t, srv, "/v1/games/g-crash/unquarantine", ""); rec.Code != http.StatusOK {
		t.Fatalf("unquarantine: %d", rec.Code)
	}
	if !feedIDs(t, srv)["g-crash"] {
		t.Error("unquarantined game must return to the feed")
	}
}

// Perf samples are filed but never indexed: no incidents, no quarantine,
// however many arrive.
func TestPerfSamplesNeverQuarantine(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "g-crash")
	perf := `{"gameId":"g-crash","runtimeVersion":"2026.10.22.1","kind":"perf","name":"PerfSample","message":"fps=60 quality=high frames=1500 dpr=2","logTail":[]}`
	for i := 0; i < quarantineGameTotalThreshold+5; i++ {
		if rec := post(t, srv, "/v1/telemetry/runtime-error", perf); rec.Code != http.StatusAccepted {
			t.Fatalf("perf report %d: want 202, got %d", i, rec.Code)
		}
	}
	game, _ := st.GetGame("g-crash")
	if game.Quarantined {
		t.Error("perf samples must never quarantine a game")
	}
	rec := get(t, srv, "/v1/telemetry/incidents")
	var inc struct {
		Incidents []incident `json:"incidents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &inc); err != nil {
		t.Fatalf("incidents json: %v", err)
	}
	if len(inc.Incidents) != 0 {
		t.Errorf("perf samples must not appear as incidents: %+v", inc.Incidents)
	}
}

// Below the threshold nothing happens.
func TestBelowThresholdNoQuarantine(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "g-crash")
	for i := 0; i < quarantineFingerprintThresh-1; i++ {
		post(t, srv, "/v1/telemetry/runtime-error", errReport)
	}
	game, _ := st.GetGame("g-crash")
	if game.Quarantined {
		t.Error("quarantined below the threshold")
	}
}

// Shadow repair: propose changes nothing live; apply publishes the fix.
func TestRepairProposeApplyShadow(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	g := seedReadyGame(st, "g-fix")
	// Break the IR: an entity far outside the world.
	g.GameIR.Scenes[0].Entities = append(g.GameIR.Scenes[0].Entities,
		gameir.Entity{ID: "lost", Kind: "pickup", Position: []float64{99999, 99999}})
	st.UpdateGame(g)

	if rec := post(t, srv, "/v1/games/g-fix/repair/propose", ""); rec.Code != http.StatusOK {
		t.Fatalf("propose: %d %s", rec.Code, rec.Body.String())
	}
	// Shadow mode: the live game is untouched until apply.
	live, _ := st.GetGame("g-fix")
	for _, e := range live.GameIR.Scenes[0].Entities {
		if e.ID == "lost" && e.Position[0] == 99999 {
			goto shadowOK
		}
	}
	t.Fatal("propose must not modify the live game")
shadowOK:
	if rec := post(t, srv, "/v1/games/g-fix/repair/apply", ""); rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body.String())
	}
	live, _ = st.GetGame("g-fix")
	for _, e := range live.GameIR.Scenes[0].Entities {
		if e.ID == "lost" {
			if e.Position[0] > 490 { // world default 390 wide + 100 margin
				t.Fatalf("applied repair did not clamp position: %v", e.Position)
			}
			return
		}
	}
	t.Fatal("lost entity vanished instead of being clamped")
}
