package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestRunSubmissionRejectsUnavailablePersistenceWithoutGhostLeaderboard(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	const payload = `{"name":"durable","score":7,"won":true,"durationMs":20000,"seed":42}`
	validDir := os.Getenv("DQ_TELEMETRY_DIR")

	// First accepted trace must survive a real Server restart.
	code, first := postRun(t, srv, gameID, payload)
	if code != http.StatusCreated {
		t.Fatalf("seed run rejected: code=%d response=%v", code, first)
	}
	firstID, ok := first["runId"].(string)
	if !ok || firstID == "" {
		t.Fatalf("accepted run missing durable locator: %v", first)
	}
	logPath := filepath.Join(validDir, "runs.jsonl")
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("accepted run missing JSONL file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("new replay trace file must be private (0600), got %04o", info.Mode().Perm())
	}

	root := t.TempDir()
	blockedParent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedLog := filepath.Join(root, "existing-dir")
	if err := os.MkdirAll(filepath.Join(blockedLog, "runs.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, blocked := range []string{blockedParent, blockedLog} {
		t.Setenv("DQ_TELEMETRY_DIR", blocked)
		code, result := postRun(t, srv, gameID, payload)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("unwritable log %q: accepted ghost run status=%d payload=%v", blocked, code, result)
		}
		if _, found := result["runId"]; found {
			t.Fatalf("failed persistence must not return a runId: %v", result)
		}
		board := getBoard(t, srv, gameID, "all")
		entries, ok := board["entries"].([]any)
		if !ok || len(entries) != 1 || entries[0].(map[string]any)["runId"] != firstID {
			t.Fatalf("failed persistence must not alter leaderboard: %v", board)
		}
		srv.runsMu.Lock()
		count := len(srv.runs)
		srv.runsMu.Unlock()
		if count != 1 {
			t.Fatalf("failed persistence published an in-memory ghost run: count=%d", count)
		}
	}

	// Recover the directory, submit again, then reload the same persisted
	// file in a new Server. Both accepted IDs must resolve uniquely.
	t.Setenv("DQ_TELEMETRY_DIR", validDir)
	code, second := postRun(t, srv, gameID, payload)
	if code != http.StatusCreated {
		t.Fatalf("retry after persistence recovery: code=%d response=%v", code, second)
	}
	secondID, ok := second["runId"].(string)
	if !ok || secondID == "" || secondID == firstID {
		t.Fatalf("retry must allocate a distinct persisted run: %v", second)
	}
	restarted := New(srv.store, pipeline.NewRunner(srv.store))
	for _, id := range []string{firstID, secondID} {
		request := httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil)
		response := httptest.NewRecorder()
		restarted.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("accepted run %s lost after restart: status=%d", id, response.Code)
		}
	}
	restarted.runsMu.Lock()
	persisted := len(restarted.runs)
	restarted.runsMu.Unlock()
	if persisted != 2 {
		t.Fatalf("persistence/restart must contain only accepted runs: got %d", persisted)
	}
}

func TestNewRunRetainsServerOwnedFullIRDigestAfterRestart(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	game, ok := srv.store.GetGame(gameID)
	if !ok {
		t.Fatal("sample inventory game missing")
	}
	expectedFull := irSHA256Of(game.GameIR)
	if len(expectedFull) != 64 {
		t.Fatalf("canonical server IR digest must have 64 hex chars: %q", expectedFull)
	}
	if decoded, err := hex.DecodeString(expectedFull); err != nil || len(decoded) != 32 {
		t.Fatalf("canonical IR digest is not SHA-256: %q, %v", expectedFull, err)
	}
	short := expectedFull[:12]
	code, posted := postRun(t, srv, gameID, `{"name":"provenance","score":6,"won":true,"durationMs":20000,"seed":42}`)
	if code != http.StatusCreated {
		t.Fatalf("run rejected: status=%d body=%v", code, posted)
	}
	runID, ok := posted["runId"].(string)
	if !ok || runID == "" {
		t.Fatalf("run identity absent: %v", posted)
	}
	for _, active := range []*Server{srv, New(srv.store, pipeline.NewRunner(srv.store))} {
		request := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID, nil)
		response := httptest.NewRecorder()
		active.Routes().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("authorized run lookup failed: %d (%s)", response.Code, response.Body.String())
		}
		var rec RunRecord
		if err := json.Unmarshal(response.Body.Bytes(), &rec); err != nil {
			t.Fatal(err)
		}
		if rec.IRHash != short || rec.IRSHA256 != expectedFull {
			t.Fatalf("new run lost server-derived digest: short=%q full=%q expected=%q",
				rec.IRHash, rec.IRSHA256, expectedFull)
		}
		if rec.Verified != nil || rec.ActualScore != nil {
			t.Fatalf("full digest must not imply independent server replay verification: %+v", rec)
		}
	}
	// Legacy JSONL has no full digest. Do not guess one from a 12-hex key.
	var legacy RunRecord
	if err := json.Unmarshal([]byte(`{"id":"legacy","gameId":"sample-rainbow-bear","irHash":"abc123def456"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.IRSHA256 != "" {
		t.Fatalf("legacy run must not obtain a fabricated full digest: %+v", legacy)
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "irSha256") {
		t.Fatalf("legacy run should omit unavailable digest: %s", raw)
	}
}

func TestReplayInputEvidenceDigestIsOrderSensitiveAndClaimsDoNotAttest(t *testing.T) {
	const gameID = "sample-rainbow-bear"
	const fullIR = "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	inputs := []RunInput{
		{Tick: 10, Type: "key", Detail: "ArrowLeft"},
		{Tick: 10, Type: "key", Detail: "ArrowRight"},
		{Tick: 11, Type: "pointer", Detail: "12,34"},
	}
	reference, err := canonicalRunInputSHA256(gameID, fullIR, 42, 20000, inputs)
	if err != nil || len(reference) != 64 {
		t.Fatalf("canonical replay digest: %q, %v", reference, err)
	}
	if raw, err := hex.DecodeString(reference); err != nil || len(raw) != 32 {
		t.Fatalf("invalid SHA-256 digest: %s, %v", reference, err)
	}
	same, err := canonicalRunInputSHA256(gameID, fullIR, 42, 20000, append([]RunInput(nil), inputs...))
	if err != nil || same != reference {
		t.Fatalf("same replay evidence must remain stable across copies: %s != %s", same, reference)
	}
	for _, example := range []struct {
		name     string
		gameID   string
		fullIR   string
		seed     uint32
		duration int
		inputs   []RunInput
	}{
		{"different-game", "another", fullIR, 42, 20000, inputs},
		{"different-IR", gameID, "1234123412341234123412341234123412341234123412341234123412341234", 42, 20000, inputs},
		{"different-seed", gameID, fullIR, 43, 20000, inputs},
		{"different-duration", gameID, fullIR, 42, 20001, inputs},
		{"missing-input", gameID, fullIR, 42, 20000, inputs[:2]},
		{"changed-key", gameID, fullIR, 42, 20000, []RunInput{{Tick: 10, Type: "key", Detail: "ArrowUp"}, inputs[1], inputs[2]}},
		{"reordered-same-tick", gameID, fullIR, 42, 20000, []RunInput{inputs[1], inputs[0], inputs[2]}},
	} {
		t.Run(example.name, func(t *testing.T) {
			digest, err := canonicalRunInputSHA256(example.gameID, example.fullIR, example.seed, example.duration, example.inputs)
			if err != nil || digest == reference {
				t.Fatalf("changed evidence improperly matches: %s == %s: %v", digest, reference, err)
			}
		})
	}
	empty, err := canonicalRunInputSHA256(gameID, fullIR, 42, 20000, nil)
	explicit, explicitErr := canonicalRunInputSHA256(gameID, fullIR, 42, 20000, []RunInput{})
	if err != nil || explicitErr != nil || empty != explicit {
		t.Fatalf("nil versus empty trace must have one canonical encoding: %s %s, %v %v", empty, explicit, err, explicitErr)
	}
	if _, err := canonicalRunInputSHA256("", fullIR, 42, 20000, inputs); err == nil {
		t.Fatal("cannot fingerprint missing game identity")
	}
	if _, err := canonicalRunInputSHA256(gameID, "short", 42, 20000, inputs); err == nil {
		t.Fatal("cannot fingerprint missing authoritative IR digest")
	}
}

func TestRunEvidenceDigestPersistedAcrossRestartWithoutTrustedScore(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	body := `{"name":"same-evidence","score":6,"won":true,"durationMs":20000,"seed":42,"inputs":[{"tick":10,"type":"key","detail":"ArrowLeft"},{"tick":10,"type":"key","detail":"ArrowRight"}]}`
	code, posted := postRun(t, srv, gameID, body)
	if code != http.StatusCreated {
		t.Fatalf("run submission: %d %v", code, posted)
	}
	id, _ := posted["runId"].(string)
	if id == "" {
		t.Fatalf("missing run ID: %v", posted)
	}
	var first RunRecord
	for index, active := range []*Server{srv, New(srv.store, pipeline.NewRunner(srv.store))} {
		rec := httptest.NewRecorder()
		active.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/runs/"+id, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("read #%d failed: %d %s", index, rec.Code, rec.Body.String())
		}
		var got RunRecord
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.ReplayInputSHA256) != 64 || len(got.IRSHA256) != 64 ||
			got.Verified != nil || got.ActualScore != nil {
			t.Fatalf("server evidence hash must not imply a verified result: %+v", got)
		}
		computed, err := canonicalRunInputSHA256(gameID, got.IRSHA256, got.Seed, got.DurationMs, got.Inputs)
		if err != nil || computed != got.ReplayInputSHA256 {
			t.Fatalf("stored evidence hash does not match admitted trace: %+v %v", got, err)
		}
		if index > 0 && (got.ReplayInputSHA256 != first.ReplayInputSHA256 || got.ID != first.ID) {
			t.Fatalf("restart changed persisted evidence identity: before %+v after %+v", first, got)
		}
		first = got
	}
	// Browser score/name/win claims are NOT inputs to the immutable evidence
	// digest; they must be independently compared by the future server engine.
	anotherCode, next := postRun(t, srv, gameID, `{"name":"another-claim","score":4,"won":false,"durationMs":20000,"seed":42,"inputs":[{"tick":10,"type":"key","detail":"ArrowLeft"},{"tick":10,"type":"key","detail":"ArrowRight"}]}`)
	if anotherCode != http.StatusCreated {
		t.Fatalf("second run: %d %+v", anotherCode, next)
	}
	nextID, _ := next["runId"].(string)
	var second RunRecord
	for _, item := range srv.runs {
		if item.ID == nextID {
			second = item
			break
		}
	}
	if second.ID == "" || second.ReplayInputSHA256 != first.ReplayInputSHA256 || second.ID == first.ID {
		t.Fatalf("same evidence must have same digest despite separate unverified claims: first %+v second %+v", first, second)
	}
	if second.Verified != nil {
		t.Fatal("replay digest must never authorize client-reported score")
	}
}

func TestRunIDRemainsUniqueAcrossProcessRestart(t *testing.T) {
	srv := newRunsServer(t)
	const gameID = "sample-rainbow-bear"
	firstCode, first := postRun(t, srv, gameID, `{"name":"first","score":7,"won":false,"durationMs":20000,"seed":42}`)
	if firstCode != http.StatusCreated {
		t.Fatalf("first run: %d, %v", firstCode, first)
	}
	firstID, _ := first["runId"].(string)
	if firstID == "" {
		t.Fatalf("first run missing ID: %v", first)
	}

	// An actual new Server reloads the persisted runs.jsonl. The previous
	// process-local counter restarted from one, duplicating the first run
	// ID on the same day; GET would then return the earlier wrong trace.
	restarted := New(srv.store, pipeline.NewRunner(srv.store))
	secondCode, second := postRun(t, restarted, gameID, `{"name":"second","score":8,"won":false,"durationMs":20000,"seed":43}`)
	if secondCode != http.StatusCreated {
		t.Fatalf("second run after restart: %d, %v", secondCode, second)
	}
	secondID, _ := second["runId"].(string)
	if secondID == firstID {
		t.Fatalf("run ID reused after restart: %q", secondID)
	}

	seen := map[string]bool{}
	for _, testCase := range []struct {
		id    string
		score float64
	}{
		{firstID, 7}, {secondID, 8},
	} {
		parts := strings.Split(testCase.id, "-")
		if len(parts) != 3 || parts[0] != "run" || len(parts[1]) != 8 || len(parts[2]) != 32 {
			t.Fatalf("unexpected run ID format: %q", testCase.id)
		}
		if _, err := hex.DecodeString(parts[2]); err != nil {
			t.Fatalf("run ID nonce must be hex: %q: %v", testCase.id, err)
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+testCase.id, nil)
		rec := httptest.NewRecorder()
		restarted.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s after restart status %d", testCase.id, rec.Code)
		}
		var run map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
			t.Fatal(err)
		}
		if run["id"] != testCase.id || run["score"] != testCase.score {
			t.Fatalf("cross-run aliasing: wanted %s score %v, got %v", testCase.id, testCase.score, run)
		}
		seen[testCase.id] = true
	}
	// Rapid same-day submissions must have distinct IDs without relying on
	// sequence state. This also detects accidental constant RNG results.
	for i := 0; i < 64; i++ {
		status, run := postRun(t, restarted, gameID, `{"name":"repeat","score":5,"won":false,"durationMs":20000,"seed":44}`)
		if status != http.StatusCreated {
			t.Fatalf("rapid run %d rejected: %d %v", i, status, run)
		}
		id, _ := run["runId"].(string)
		if id == "" || seen[id] {
			t.Fatalf("new run ID missing/duplicated after %d submissions: %q", i, id)
		}
		seen[id] = true
	}
	restarted.runsMu.Lock()
	stored := len(restarted.runs)
	restarted.runsMu.Unlock()
	if stored != len(seen) {
		t.Fatalf("persistence/restart index mismatch: %d records, %d distinct IDs", stored, len(seen))
	}
}

func TestReplayPointerGrammarParityWithTrustedRecorder(t *testing.T) {
	srv := newRunsServer(t)
	cases := []struct {
		name   string
		typ    string
		detail string
		want   int
	}{
		{"zero", "pointer", "0,0", http.StatusCreated},
		{"negative_and_max", "pointer", "-12,1000000", http.StatusCreated},
		{"signed_boundaries", "pointer", "1000000,-1000000", http.StatusCreated},
		{"empty_pointer", "pointer", "", http.StatusBadRequest},
		{"one_coordinate", "pointer", "0", http.StatusBadRequest},
		{"extra_coordinate", "pointer", "1,2,3", http.StatusBadRequest},
		{"missing_coordinate", "pointer", ",2", http.StatusBadRequest},
		{"not_a_number", "pointer", "NaN,0", http.StatusBadRequest},
		{"infinity", "pointer", "Infinity,3", http.StatusBadRequest},
		{"decimal", "pointer", "1.2,3", http.StatusBadRequest},
		{"leading_zero", "pointer", "01,2", http.StatusBadRequest},
		{"negative_zero", "pointer", "-0,2", http.StatusBadRequest},
		{"plus_sign", "pointer", "+1,2", http.StatusBadRequest},
		{"above_limit", "pointer", "1000001,2", http.StatusBadRequest},
		{"below_limit", "pointer", "-1000001,0", http.StatusBadRequest},
		{"whitespace", "pointer", " 1,2", http.StatusBadRequest},
		{"release_empty", "pointerup", "", http.StatusCreated},
		{"release_forged_detail", "pointerup", "1,2", http.StatusBadRequest},
		{"key_binding", "key", "ArrowLeft", http.StatusCreated},
		{"key_release", "keyup", "ArrowLeft", http.StatusCreated},
		{"legacy_empty_key", "key", "", http.StatusCreated},
	}
	accepted := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{
				"name": "ReplayShape", "score": 5, "won": false,
				"durationMs": 20000, "seed": 42,
				"inputs": []RunInput{{Tick: 7, Type: tc.typ, Detail: tc.detail}},
			})
			if err != nil {
				t.Fatal(err)
			}
			status, _ := postRun(t, srv, "sample-rainbow-bear", string(payload))
			if status != tc.want {
				t.Fatalf("replay detail %q type %s: got %d, want %d", tc.detail, tc.typ, status, tc.want)
			}
			if tc.want == http.StatusCreated {
				accepted++
			}
		})
	}
	srv.runsMu.Lock()
	stored := len(srv.runs)
	srv.runsMu.Unlock()
	if stored != accepted {
		t.Fatalf("rejected pointer evidence persisted: got %d records, want %d", stored, accepted)
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

func TestHistoricalClientVerdictsNeverAppearAsServerAttestation(t *testing.T) {
	srv := newRunsServer(t)
	const gid = "sample-rainbow-bear"
	game, ok := srv.store.GetGame(gid)
	if !ok {
		t.Fatal("published test game missing")
	}
	legacyVerdict := true
	legacyScore := 12
	srv.runsMu.Lock()
	srv.runs = append(srv.runs, RunRecord{
		ID: "legacy-client-verified", GameID: gid, IRHash: irHashOf(game.GameIR),
		Name: "Legacy", Score: 12, Won: true, DurationMs: 20000,
		Seed: 42, Inputs: []RunInput{{Tick: 10, Type: "key", Detail: "ArrowLeft"}},
		At: time.Now(), Verified: &legacyVerdict, ActualScore: &legacyScore,
	})
	srv.runsMu.Unlock()

	board := getBoard(t, srv, gid, "week")
	entries := board["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("legacy rank should remain visible but untrusted: %v", entries)
	}
	listed := entries[0].(map[string]any)
	if _, leaked := listed["verified"]; leaked {
		t.Fatalf("historical client verdict leaked into leaderboard API: %v", listed)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/legacy-client-verified", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorized public replay retrieval failed: %d", rec.Code)
	}
	var record map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if _, leaked := record["verified"]; leaked {
		t.Fatalf("historical client verdict leaked into recorded-run response: %v", record)
	}
	if _, leaked := record["actualScore"]; leaked {
		t.Fatalf("untrusted historical actualScore leaked into replay response: %v", record)
	}
	if record["score"] != 12.0 || len(record["inputs"].([]any)) != 1 {
		t.Fatalf("safe replay seed/input payload was silently lost: %v", record)
	}
	// Do not destroy historical records without an explicit, reviewed
	// migration: the read API must mask on a copy, not mutate storage.
	srv.runsMu.Lock()
	stored := srv.runs[0]
	srv.runsMu.Unlock()
	if stored.Verified == nil || !*stored.Verified ||
		stored.ActualScore == nil || *stored.ActualScore != 12 {
		t.Fatalf("historical verdict must remain for audit: %+v", stored)
	}
}
