package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/inventory"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func newRunsServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	st := store.NewMemoryStore()
	if err := inventory.Load(st, "../../inventory"); err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	return New(st, pipeline.NewRunner(st))
}

func postRun(t *testing.T, srv *Server, gameID, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/games/"+gameID+"/runs", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func getBoard(t *testing.T, srv *Server, gameID, window string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/games/"+gameID+"/leaderboard?window="+window, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("board %s: got %d (%s)", gameID, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("board json: %v", err)
	}
	return out
}

func TestRunsSubmitBoardAndVerify(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	trace := `"inputs":[{"tick":10,"type":"pointer","detail":"195,700"},{"tick":40,"type":"pointer","detail":"220,700"}]`
	// Bobo's declared board is time/asc over won runs: fastest win tops it.
	code, resp := postRun(t, srv, gid, `{"name":"KidA","score":12,"won":true,"durationMs":25000,"seed":42,`+trace+`}`)
	if code != http.StatusCreated || resp["rank"] != 1.0 {
		t.Fatalf("first run: code %d resp %v", code, resp)
	}
	runID, _ := resp["runId"].(string)
	if runID == "" {
		t.Fatalf("no runId in %v", resp)
	}
	code, resp = postRun(t, srv, gid, `{"name":"KidB","score":13,"won":true,"durationMs":21000,"seed":43}`)
	if code != http.StatusCreated || resp["rank"] != 1.0 {
		t.Fatalf("faster win should rank 1: code %d resp %v", code, resp)
	}
	board := getBoard(t, srv, gid, "week")
	if board["metric"] != "time" || board["order"] != "asc" {
		t.Fatalf("board metric/order: %v", board)
	}
	entries := board["entries"].([]any)
	if len(entries) != 2 || entries[0].(map[string]any)["name"] != "KidB" {
		t.Fatalf("board order wrong: %v", entries)
	}
	if _, present := entries[0].(map[string]any)["verified"]; present {
		t.Fatalf("unverified run should omit the verified flag: %v", entries[0])
	}
	// The run is fetchable with its trace for replay.
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	var run map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &run)
	if run["seed"] != 42.0 || len(run["inputs"].([]any)) != 2 {
		t.Fatalf("run fetch: %v", run)
	}
	// Verification lands once; the board then shows verified.
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/verification", strings.NewReader(`{"match":true,"actualScore":12}`))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verification: %d", rec.Code)
	}
	board = getBoard(t, srv, gid, "week")
	entries = board["entries"].([]any)
	for _, e := range entries {
		em := e.(map[string]any)
		if em["runId"] == runID {
			if em["verified"] != true {
				t.Fatalf("verified flag not on board: %v", em)
			}
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/verification", strings.NewReader(`{"match":false,"actualScore":0}`))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	var vresp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &vresp)
	if vresp["alreadyRecorded"] != true || vresp["verified"] != true {
		t.Fatalf("second verdict must not rewrite history: %v", vresp)
	}
}

func TestRunsRejections(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	if code, _ := postRun(t, srv, gid, `{"name":"X","score":100000,"won":true,"durationMs":20000,"seed":1}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("implausible score: got %d, want 422", code)
	}
	if code, _ := postRun(t, srv, gid, `{"name":"X","score":5,"won":false,"durationMs":20000,"seed":1,"irHash":"deadbeef0000"}`); code != http.StatusConflict {
		t.Fatalf("wrong version: got %d, want 409", code)
	}
	if code, _ := postRun(t, srv, gid, `{"name":"X","score":5,"won":false,"durationMs":500,"seed":1}`); code != http.StatusBadRequest {
		t.Fatalf("short duration: got %d, want 400", code)
	}
	if code, _ := postRun(t, srv, gid, `{"name":"X","score":5,"won":false,"durationMs":20000,"seed":1,"inputs":[{"tick":1,"type":"teleport","detail":""}]}`); code != http.StatusBadRequest {
		t.Fatalf("bad input type: got %d, want 400", code)
	}
	if code, _ := postRun(t, srv, "no-such-game", `{"name":"X","score":5,"won":false,"durationMs":20000,"seed":1}`); code != http.StatusNotFound {
		t.Fatalf("unknown game: got %d, want 404", code)
	}
}

func TestRunsNameFilterAndTimeBoard(t *testing.T) {
	srv := newRunsServer(t)
	code, resp := postRun(t, srv, "sample-rainbow-bear", `{"name":"little shit","score":12,"won":true,"durationMs":20000,"seed":1}`)
	if code != http.StatusCreated || resp["nameFiltered"] != true {
		t.Fatalf("filter: code %d resp %v", code, resp)
	}
	board := getBoard(t, srv, "sample-rainbow-bear", "week")
	if board["entries"].([]any)[0].(map[string]any)["name"] != "Player" {
		t.Fatalf("filtered name on board: %v", board["entries"])
	}
	// Escape's board is time/asc over WON runs only.
	gid := "escape-from-injection"
	postRun(t, srv, gid, `{"name":"A","score":15,"won":true,"durationMs":45000,"seed":1}`)
	postRun(t, srv, gid, `{"name":"B","score":15,"won":true,"durationMs":38000,"seed":2}`)
	postRun(t, srv, gid, `{"name":"C","score":3,"won":false,"durationMs":20000,"seed":3}`)
	board = getBoard(t, srv, gid, "week")
	if board["metric"] != "time" || board["order"] != "asc" {
		t.Fatalf("escape board metric: %v", board)
	}
	entries := board["entries"].([]any)
	if len(entries) != 2 || entries[0].(map[string]any)["name"] != "B" {
		t.Fatalf("time board: %v", entries)
	}
}

func TestRunsWeeklyWindow(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	game, _ := srv.store.GetGame(gid)
	srv.runsMu.Lock()
	srv.runs = append(srv.runs, RunRecord{
		ID: "run-old", GameID: gid, IRHash: irHashOf(game.GameIR),
		Name: "Old", Score: 11, Won: true, DurationMs: 20000, At: time.Now().Add(-8 * 24 * time.Hour),
	})
	srv.runsMu.Unlock()
	if n := len(getBoard(t, srv, gid, "week")["entries"].([]any)); n != 0 {
		t.Fatalf("week board should exclude the 8-day-old run, has %d", n)
	}
	if n := len(getBoard(t, srv, gid, "all")["entries"].([]any)); n != 1 {
		t.Fatalf("all board should include it, has %d", n)
	}
}
