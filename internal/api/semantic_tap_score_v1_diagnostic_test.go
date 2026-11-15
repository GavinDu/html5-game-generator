package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/replaytrace"
	"doodlequest/internal/semanticreplay"
)

func diagnosticTapIR() *gameir.Document {
	return &gameir.Document{
		IRVersion: gameir.Version,
		Game: gameir.Game{
			ID: "creator-authored-ir-game", Title: "Published Tap Diagnostic",
			EntryScene: "main", Orientation: "any",
		},
		Scenes:        []gameir.Scene{{ID: "main"}},
		InputBindings: []gameir.InputBinding{{Action: "tap", Input: "tap"}},
		Rules: []gameir.Rule{{
			ID:   "score",
			When: gameir.Condition{Event: "input", InputAction: "tap"},
			Actions: []gameir.Action{
				{Type: "add_score", Value: float64(4)},
				{Type: "end_game", Value: true},
			},
		}},
	}
}

func TestSemanticTapScoreDiagnosticRequiresPublishedRecordAndNeverWritesScore(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true") // In-memory test identities only.
	srv := newRunsServer(t)
	const id = "sample-rainbow-bear"
	game, exists := srv.store.GetGame(id)
	if !exists {
		t.Fatal("missing test game")
	}
	game.GameIR = diagnosticTapIR()
	game.Status = model.GameReady
	game.Visibility = model.VisibilityPublic
	game.ModerationStatus = "approved"
	srv.store.UpdateGame(game)
	if err := semanticreplay.SupportedIR(game.GameIR); err != nil {
		t.Fatalf("test published Go diagnostic IR invalid: %v", err)
	}
	full := irSHA256Of(game.GameIR)
	path := "/v1/games/" + id + "/semantic-tap-score-v1/diagnose?irSha256=" +
		url.QueryEscape(full) + "&seed=42"
	trace := replaytrace.Trace{
		Schema: replaytrace.SchemaV2, DurationTicks: 12,
		Events: []replaytrace.Event{
			{Tick: 2, Ordinal: 0, Action: "tap", Device: "pointer",
				Phase: "down", Position: &replaytrace.Position{X: 10, Y: 20}},
			{Tick: 2, Ordinal: 1, Action: "tap", Device: "pointer", Phase: "up"},
			{Tick: 3, Ordinal: 0, Action: "tap", Device: "pointer",
				Phase: "down", Position: &replaytrace.Position{X: 30, Y: 40}},
			{Tick: 3, Ordinal: 1, Action: "tap", Device: "pointer", Phase: "up"},
		},
	}
	raw, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, uri string, body []byte, viewer, mime string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, uri, bytes.NewReader(body))
		req.Header.Set("Content-Type", mime)
		if viewer != "" {
			req.Header.Set("X-User-ID", viewer)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	before := len(srv.runs)
	result := call(http.MethodPost, path, raw, "", "application/json")
	if result.Code != http.StatusOK {
		t.Fatalf("authorized diagnostic refused HTTP %d: %s", result.Code, result.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(result.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["schema"] != "semantic-tap-score-diagnostic-v1" ||
		decoded["gameId"] != id || decoded["irSha256"] != full ||
		decoded["proofVersion"] != semanticreplay.ProofVersion ||
		decoded["score"] != float64(4) || decoded["terminal"] != "win" ||
		decoded["downEvents"] != float64(1) ||
		decoded["actionsApplied"] != float64(2) ||
		decoded["durationTicks"] != float64(12) ||
		decoded["eventCount"] != float64(4) ||
		decoded["evidenceStatus"] != "unverified" ||
		decoded["rankingScope"] != "none" || decoded["attested"] != false ||
		len(decoded["traceSha256"].(string)) != 64 ||
		len(decoded["stateSha256"].(string)) != 64 {
		t.Fatalf("diagnostic contract drift: %+v", decoded)
	}
	if _, ok := decoded["verified"]; ok {
		t.Fatal("public Go diagnostic must never include a verified field")
	}
	if _, ok := decoded["runId"]; ok {
		t.Fatal("diagnostic must never mint a run ID")
	}
	if result.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(result.Header().Get("Vary"), "Authorization") ||
		!strings.Contains(result.Header().Get("Vary"), "X-User-ID") {
		t.Fatal("published diagnosis response missing private, auth-vary cache controls")
	}
	repeat := call(http.MethodPost, path, raw, "", "application/json; charset=utf-8")
	if repeat.Code != http.StatusOK || repeat.Body.String() != result.Body.String() {
		t.Fatalf("same authorized diagnostic did not repeat: status=%d body=%s", repeat.Code, repeat.Body.String())
	}
	if len(srv.runs) != before {
		t.Fatal("diagnostics must not create persistent run or score")
	}

	invalid := []struct {
		name   string
		uri    string
		body   []byte
		mime   string
		status int
	}{
		{"stale-ir", strings.Replace(path, full, strings.Repeat("0", 64), 1), raw, "application/json", http.StatusConflict},
		{"missing-hash", "/v1/games/" + id + "/semantic-tap-score-v1/diagnose?seed=42", raw, "application/json", http.StatusBadRequest},
		{"duplicate-seed", path + "&seed=1", raw, "application/json", http.StatusBadRequest},
		{"extra-query", path + "&verified=true", raw, "application/json", http.StatusBadRequest},
		{"noncanonical-seed", strings.Replace(path, "seed=42", "seed=042", 1), raw, "application/json", http.StatusBadRequest},
		{"seed-overflow", strings.Replace(path, "seed=42", "seed=4294967296", 1), raw, "application/json", http.StatusBadRequest},
		{"no-body", path, []byte{}, "application/json", http.StatusBadRequest},
		{"invalid-type", path, raw, "text/html", http.StatusUnsupportedMediaType},
		{"invalid-schema", path, []byte(`{"schema":"semantic-input-trace-v1","durationTicks":12,"events":[]}`), "application/json", http.StatusBadRequest},
		{"unknown-key", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":12,"events":[],"verified":true}`), "application/json", http.StatusBadRequest},
		{"duplicate-key", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":12,"events":[],"events":[]}`), "application/json", http.StatusBadRequest},
		{"case-aliased-events", path, []byte(`{"schema":"semantic-input-trace-v2","durationTicks":12,"events":[],"Events":[]}`), "application/json", http.StatusBadRequest},
		{"oversized", path, bytes.Repeat([]byte(" "), replaytrace.MaxEncodedTraceBytes+1), "application/json", http.StatusRequestEntityTooLarge},
		{"wrong-record", strings.Replace(path, "/"+id+"/", "/not-a-published-game/", 1), raw, "application/json", http.StatusNotFound},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(http.MethodPost, tc.uri, tc.body, "", tc.mime)
			if rec.Code != tc.status {
				t.Fatalf("negative got HTTP %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("negative must remain private, no-store")
			}
		})
	}
	if len(srv.runs) != before {
		t.Fatal("negative diagnostic changed persistent run state")
	}
	// Unsupported published games fail all-or-nothing: no misleading
	// partial score from silently ignored physics or entity conditions.
	game.GameIR = diagnosticTapIR()
	game.GameIR.InputBindings[0].Input = "drag"
	srv.store.UpdateGame(game)
	unsupported := call(http.MethodPost, path, raw, "", "application/json")
	if unsupported.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported profile admitted: HTTP %d %s", unsupported.Code, unsupported.Body.String())
	}
	game.GameIR = diagnosticTapIR()
	game.Visibility = model.VisibilityPrivate
	game.OwnerID = "real-publisher"
	srv.store.UpdateGame(game)
	for _, viewer := range []string{"", "non-owner"} {
		rec := call(http.MethodPost, path, raw, viewer, "application/json")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("private diagnostic exposed to viewer %q: HTTP %d", viewer, rec.Code)
		}
	}
	owner := call(http.MethodPost, path, raw, "real-publisher", "application/json")
	if owner.Code != http.StatusOK {
		t.Fatalf("authorized creator diagnostic denied: %d %s", owner.Code, owner.Body.String())
	}
	if len(srv.runs) != before {
		t.Fatal("private owner diagnostic unexpectedly wrote run state")
	}
}

func TestTapScoreDiagnosticRecordIDDomainsAreNotAuthoredIRGameIDs(t *testing.T) {
	ir := diagnosticTapIR()
	trace := replaytrace.Trace{Schema: replaytrace.SchemaV2, DurationTicks: 8}
	a, err := semanticreplay.ReplayDiagnosticForRecord(ir, "server-record-A", trace, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := semanticreplay.ReplayDiagnosticForRecord(ir, "server-record-B", trace, 1)
	if err != nil {
		t.Fatal(err)
	}
	if a.TraceSHA256 == b.TraceSHA256 || a.StateSHA256 == b.StateSHA256 ||
		a.Score != b.Score || a.Terminal != b.Terminal ||
		a.Attested || b.Attested {
		t.Fatalf("record ID must be fingerprint domain, not gameplay side effect: A=%+v B=%+v", a, b)
	}
	if _, err := semanticreplay.ReplayDiagnosticForRecord(ir, "", trace, 1); err == nil {
		t.Fatal("empty authorized record ID accepted")
	}
}


func TestSemanticTapScoreDiagnosticCapacityFailsClosedBeforeExpensiveTraceParsing(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	srv := newRunsServer(t)
	const id = "sample-rainbow-bear"
	game, ok := srv.store.GetGame(id)
	if !ok {
		t.Fatal("missing published game")
	}
	game.GameIR = diagnosticTapIR()
	game.Status = model.GameReady
	game.Visibility = model.VisibilityPublic
	game.ModerationStatus = "approved"
	srv.store.UpdateGame(game)
	uri := "/v1/games/" + id + "/semantic-tap-score-v1/diagnose?irSha256=" +
		irSHA256Of(game.GameIR) + "&seed=0"
	before := len(srv.runs)
	if len(semanticTapDiagnosticSlots) != 0 {
		t.Fatal("diagnostic capacity unexpectedly occupied before test")
	}
	for i := 0; i < cap(semanticTapDiagnosticSlots); i++ {
		semanticTapDiagnosticSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(semanticTapDiagnosticSlots); i++ {
			<-semanticTapDiagnosticSlots
		}
	}()
	request := func(viewer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, uri, strings.NewReader(
			`{"schema":"semantic-input-trace-v2","durationTicks":4,"events":[]}`,
		))
		req.Header.Set("Content-Type", "application/json")
		if viewer != "" {
			req.Header.Set("X-User-ID", viewer)
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	busy := request("")
	if busy.Code != http.StatusTooManyRequests ||
		busy.Header().Get("Retry-After") != "1" ||
		busy.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(busy.Header().Get("Vary"), "Authorization") {
		t.Fatalf("bounded diagnostic capacity did not fail privately with 429: HTTP %d headers=%v",
			busy.Code, busy.Header())
	}
	if len(srv.runs) != before {
		t.Fatal("busy semantic replay unexpectedly created a run")
	}
	// Saturation must NOT reveal the existence of private games to guests;
	// deliverableGame authorization happens before capacity acquisition.
	game.Visibility = model.VisibilityPrivate
	game.OwnerID = "private-creator"
	srv.store.UpdateGame(game)
	denied := request("")
	if denied.Code != http.StatusNotFound {
		t.Fatalf("private game leaked capacity state to guest: HTTP %d", denied.Code)
	}
}
