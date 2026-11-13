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
	// A browser (even an authenticated browser replaying the actual seed)
	// must not be able to attest its own score. Until a server-owned replay
	// verifier ships, every claimed verdict is denied and never stored.
	for _, tc := range []struct {
		name string
		body string
	}{
		{"forged_match", `{"match":true,"actualScore":12}`},
		{"negative_match", `{"match":false,"actualScore":0}`},
		{"invalid_json", `{"match":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runID+"/verification", strings.NewReader(tc.body))
			req.Header.Set("X-User-ID", "another-player")
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("untrusted verdict accepted: status %d body %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
				t.Fatalf("verification rejection must be non-cacheable: %v", rec.Header())
			}
		})
	}
	board = getBoard(t, srv, gid, "week")
	entries = board["entries"].([]any)
	for _, e := range entries {
		em := e.(map[string]any)
		if em["runId"] == runID {
			if _, present := em["verified"]; present {
				t.Fatalf("client verdict must not set verified flag: %v", em)
			}
		}
	}
	srv.runsMu.Lock()
	var stored *RunRecord
	for i := range srv.runs {
		if srv.runs[i].ID == runID {
			copy := srv.runs[i]
			stored = &copy
			break
		}
	}
	srv.runsMu.Unlock()
	if stored == nil || stored.Verified != nil || stored.ActualScore != nil {
		t.Fatalf("untrusted verdict modified persisted run: %+v", stored)
	}
	// Do not reveal whether a guessed run ID exists through this retired
	// endpoint; the response is independent of the run's presence.
	req = httptest.NewRequest(http.MethodPost, "/v1/runs/missing-id/verification", strings.NewReader(`{"match":true}`))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unknown-run verdict must also fail closed: %d", rec.Code)
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
	// Escape follows the authored caught ending: no fake wins. Until the
	// level-then-score leaderboard has a first-class ranking metric, rank
	// real caught runs by score descending rather than fastest fake escape.
	gid := "escape-from-injection"
	postRun(t, srv, gid, `{"name":"A","score":15,"won":false,"durationMs":45000,"seed":1}`)
	postRun(t, srv, gid, `{"name":"B","score":25,"won":false,"durationMs":38000,"seed":2}`)
	postRun(t, srv, gid, `{"name":"C","score":3,"won":false,"durationMs":20000,"seed":3}`)
	board = getBoard(t, srv, gid, "week")
	if board["metric"] != "score" || board["order"] != "desc" {
		t.Fatalf("escape board metric: %v", board)
	}
	entries := board["entries"].([]any)
	if len(entries) != 3 || entries[0].(map[string]any)["name"] != "B" {
		t.Fatalf("caught-run score board: %v", entries)
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

func TestRunsRejectNonMonotonicReplayTicks(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	const prefix = `{"name":"TickOrder","score":5,"won":false,"durationMs":20000,"seed":42,"inputs":`
	cases := []struct {
		name   string
		inputs string
		want   int
	}{
		{"same_tick_allowed", `[{"tick":10,"type":"pointer","detail":"10,20"},{"tick":10,"type":"pointerup","detail":""}]`, http.StatusCreated},
		{"ascending_allowed", `[{"tick":10,"type":"key","detail":"ArrowLeft"},{"tick":11,"type":"keyup","detail":"ArrowLeft"}]`, http.StatusCreated},
		{"backwards_rejected", `[{"tick":10,"type":"pointer","detail":"10,20"},{"tick":9,"type":"pointerup","detail":""}]`, http.StatusBadRequest},
		{"negative_rejected", `[{"tick":0,"type":"pointer","detail":"10,20"},{"tick":-1,"type":"pointerup","detail":""}]`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := postRun(t, srv, gid, prefix+tc.inputs+"}")
			if code != tc.want {
				t.Fatalf("input trace validation status = %d, want %d", code, tc.want)
			}
		})
	}
	// This game's public time leaderboard omits losing runs, so inspect
	// persistent run admission rather than equating board entries with posts.
	srv.runsMu.Lock()
	stored := len(srv.runs)
	srv.runsMu.Unlock()
	if stored != 2 {
		t.Fatalf("rejected replay traces must not be stored: got %d, want 2", stored)
	}
}

func TestRunDetailBudgetMatchesJavaScriptUTF16(t *testing.T) {
	srv := newRunsServer(t)
	gid := "sample-rainbow-bear"
	cases := []struct {
		name   string
		detail string
		want   int
	}{
		{"empty", "", http.StatusCreated},
		{"ascii_exact_128", strings.Repeat("x", 128), http.StatusCreated},
		{"ascii_over_129", strings.Repeat("x", 129), http.StatusBadRequest},
		{"supplementary_exact_128", strings.Repeat("🎮", 64), http.StatusCreated},
		{"supplementary_over_130", strings.Repeat("🎮", 65), http.StatusBadRequest},
		{"mixed_boundary", strings.Repeat("x", 126) + "🎮", http.StatusCreated},
		{"mixed_over", strings.Repeat("x", 127) + "🎮", http.StatusBadRequest},
	}
	accepted := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"name": "ReplayDetail", "score": 5, "won": false,
				"durationMs": 20000, "seed": 42,
				"inputs": []RunInput{{Tick: 10, Type: "key", Detail: tc.detail}},
			})
			if err != nil {
				t.Fatal(err)
			}
			code, _ := postRun(t, srv, gid, string(payload))
			if code != tc.want {
				t.Fatalf("POST detail validation = %d, want %d", code, tc.want)
			}
			if tc.want == http.StatusCreated {
				accepted++
			}
		})
	}
	// Losing runs need not appear on the public time/wins leaderboard;
	// validate the persistence boundary itself, not board presentation.
	srv.runsMu.Lock()
	stored := len(srv.runs)
	srv.runsMu.Unlock()
	if stored != accepted {
		t.Fatalf("oversized detail must never be stored: %d records, want %d", stored, accepted)
	}
}

func TestAuthenticatedClientCannotSelfAttestRun(t *testing.T) {
	srv := newRunsServer(t)
	const secret = "test-replay-attestation-only-not-production"
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	token := authTokenForTest("authenticated-attacker", time.Now().Add(time.Hour), secret)
	srv.runsMu.Lock()
	srv.runs = append(srv.runs, RunRecord{
		ID: "attestation-target", GameID: "sample-rainbow-bear",
		Score: 12, Won: true, Seed: 42, DurationMs: 20000,
	})
	srv.runsMu.Unlock()
	for _, tc := range []struct {
		name string
		url  string
		body string
	}{
		{"forged_positive", "/v1/runs/attestation-target/verification", `{"match":true,"actualScore":12}`},
		{"forged_negative", "/v1/runs/attestation-target/verification", `{"match":false,"actualScore":0}`},
		{"guessed_id", "/v1/runs/no-such-run/verification", `{"match":true,"actualScore":99999}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("valid bearer must not authorize self-attestation: %d: %s",
					rec.Code, rec.Body.String())
			}
		})
	}
	srv.runsMu.Lock()
	stored := srv.runs[0]
	srv.runsMu.Unlock()
	if stored.Verified != nil || stored.ActualScore != nil {
		t.Fatalf("authenticated browser changed verification state: %+v", stored)
	}
}

func TestTrustedRuntimeNeverSelfPostsReplayVerdict(t *testing.T) {
	// The Go handler is the authoritative security boundary. This additional
	// shipped-asset contract guards against accidentally reintroducing a
	// client-side POST, which is both misleading and wastes network work.
	if strings.Contains(pixiRuntimeJS, "/verification") {
		t.Fatal("trusted Pixi runtime must never POST a client-owned replay verdict")
	}
	if !strings.Contains(pixiRuntimeJS, "data.verified === true") {
		t.Fatal("native verified-result notifications require an explicit server verdict")
	}
}
