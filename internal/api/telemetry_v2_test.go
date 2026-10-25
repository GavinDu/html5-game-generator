package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func postEvent(t *testing.T, srv *Server, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/telemetry/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec.Code
}

func TestSocialEventIdempotency(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(t.TempDir(), "social-events.jsonl"))
	srv := newTestServer()
	seedReadyGame(srv.store, "event-idempotency-game")

	send := func(user, eventID, kind string) int {
		t.Helper()
		body := fmt.Sprintf(`{"event_id":%q,"game_id":"event-idempotency-game","kind":%q}`, eventID, kind)
		req := httptest.NewRequest(http.MethodPost, "/v1/telemetry/social-event", strings.NewReader(body))
		req.Header.Set("X-User-ID", user)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send("viewer-a", "stable-event-1", "impression"); code != http.StatusAccepted {
		t.Fatalf("first event: got %d", code)
	}
	// A retry may arrive with a different payload after a client-side
	// timeout; the first accepted event remains authoritative.
	if code := send("viewer-a", "stable-event-1", "share"); code != http.StatusAccepted {
		t.Fatalf("duplicate event: got %d", code)
	}
	// Event IDs are scoped to the actor, so another viewer can emit the same ID.
	if code := send("viewer-b", "stable-event-1", "impression"); code != http.StatusAccepted {
		t.Fatalf("other viewer event: got %d", code)
	}
	if code := send("viewer-a", strings.Repeat("x", 129), "impression"); code != http.StatusBadRequest {
		t.Fatalf("oversized event ID: got %d, want 400", code)
	}

	srv.socialMu.RLock()
	defer srv.socialMu.RUnlock()
	if len(srv.events) != 2 {
		t.Fatalf("got %d events, want 2 after retry deduplication", len(srv.events))
	}
	if srv.events[0].EventID != "stable-event-1" || srv.events[0].Kind != "impression" {
		t.Fatalf("duplicate overwrote original event: %+v", srv.events[0])
	}
	if srv.events[1].UserID != "viewer-b" {
		t.Fatalf("event ID collision across users: %+v", srv.events[1])
	}
}

func TestTelemetryEventsAcceptedAndSummarized(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "telemetry-test-token")
	srv := newTestServer()
	gid := "sample-rainbow-bear"
	if code := postEvent(t, srv, fmt.Sprintf(`{"gameId":%q,"kind":"session_start","seed":42,"shell":true,"bootMs":800}`, gid)); code != http.StatusAccepted {
		t.Fatalf("session_start: got %d", code)
	}
	if code := postEvent(t, srv, fmt.Sprintf(`{"gameId":%q,"kind":"perf_window","frames":300,"buckets":[100,150,30,10,5,5],"longestFrameMs":80}`, gid)); code != http.StatusAccepted {
		t.Fatalf("perf_window: got %d", code)
	}
	if code := postEvent(t, srv, fmt.Sprintf(`{"gameId":%q,"kind":"milestone","milestone":"first_reward","ms":4200}`, gid)); code != http.StatusAccepted {
		t.Fatalf("milestone: got %d", code)
	}
	if code := postEvent(t, srv, fmt.Sprintf(`{"gameId":%q,"kind":"run_end","won":true,"score":12,"durationMs":45000,"inputs":30,"deadInputs":3}`, gid)); code != http.StatusAccepted {
		t.Fatalf("run_end: got %d", code)
	}
	if code := postEvent(t, srv, `{"gameId":"x","kind":"bogus"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown kind: got %d, want 400", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/telemetry/perf?game="+gid, nil)
	req.Header.Set("X-Telemetry-Admin-Token", "telemetry-test-token")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("perf summary: got %d", rec.Code)
	}
	var out struct {
		Games map[string]perfSummary `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	p, ok := out.Games[gid]
	if !ok {
		t.Fatalf("game missing from summary: %s", rec.Body.String())
	}
	if p.Sessions != 1 || p.Runs != 1 || p.WinRate != 1 || p.Frames != 300 || p.PerfWindows != 1 {
		t.Fatalf("summary wrong: %+v", p)
	}
	if p.FirstRewardMedianMs != 4200 {
		t.Fatalf("first reward median = %d, want 4200", p.FirstRewardMedianMs)
	}
	// buckets: 300 frames, 10 above 33ms -> jank 10/300; cumulative
	// 100, 250 (83%), 280 (93%), 290 (96.7%) -> p95 lands at the <=33
	// bucket edge.
	if p.JankRate < 0.03 || p.JankRate > 0.04 {
		t.Fatalf("jank rate = %v, want ~0.033", p.JankRate)
	}
	if p.P95FrameMs != 33 {
		t.Fatalf("p95 = %d, want 33", p.P95FrameMs)
	}
	if p.DeadInputRate != 0.1 {
		t.Fatalf("dead input rate = %v, want 0.1", p.DeadInputRate)
	}
}

// The Phase-A rate amendment: a busy game with many sessions and a low
// error RATE must not quarantine (the old count rule fired at 10
// reports regardless of volume); a small game whose errors are a
// quarter of its sessions must.
func TestRateBasedQuarantine(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "g-busy")
	seedReadyGame(st, "g-small")

	postErr := func(gid string, n int) {
		for i := 0; i < n; i++ {
			// Spread over 3 fingerprints so no single one reaches the
			// fingerprint threshold — this test isolates the rate rule.
			srv.observeTelemetry(teleEvent{Fingerprint: fmt.Sprintf("fp%d", i%3), GameID: gid, Kind: "render", Name: "E", Message: "boom", At: time.Now().UTC()})
		}
	}
	postSessions := func(gid string, n int) {
		for i := 0; i < n; i++ {
			if code := postEvent(t, srv, fmt.Sprintf(`{"gameId":%q,"kind":"session_start"}`, gid)); code != http.StatusAccepted {
				t.Fatalf("session post: %d", code)
			}
		}
	}

	// Busy: 200 sessions, 12 error reports (6% rate) -> NO quarantine.
	postSessions("g-busy", 200)
	postErr("g-busy", 12)
	if g, _ := st.GetGame("g-busy"); g.Quarantined {
		t.Fatalf("busy game quarantined at a 6%% error rate: %s", g.QuarantineReason)
	}

	// Small: 30 sessions, 10 reports (33% rate) -> quarantine.
	postSessions("g-small", 30)
	postErr("g-small", 10)
	if g, _ := st.GetGame("g-small"); !g.Quarantined {
		t.Fatal("small game at a 33% error rate was not quarantined")
	}
}

func TestTelemetryV2DeduplicatesEventIDsAndRejectsUnknownSchema(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "telemetry-test-token")
	srv := newTestServer()
	body := `{"schemaVersion":2,"eventId":"session-a:1","sessionId":"session-a","gameId":"dedupe-game","kind":"session_start"}`
	if code := postEvent(t, srv, body); code != http.StatusAccepted {
		t.Fatalf("first event: got %d", code)
	}
	if code := postEvent(t, srv, body); code != http.StatusAccepted {
		t.Fatalf("duplicate event should be safely acknowledged: got %d", code)
	}
	if code := postEvent(t, srv, `{"schemaVersion":3,"eventId":"session-a:2","sessionId":"session-a","gameId":"dedupe-game","kind":"session_start"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown schema: got %d, want 400", code)
	}
	if code := postEvent(t, srv, `{"schemaVersion":2,"eventId":"no-session","gameId":"dedupe-game","kind":"session_start"}`); code != http.StatusBadRequest {
		t.Fatalf("event ID without session ID: got %d, want 400", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/telemetry/perf?game=dedupe-game", nil)
	req.Header.Set("X-Telemetry-Admin-Token", "telemetry-test-token")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	var out struct {
		Games map[string]perfSummary `json:"games"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if got := out.Games["dedupe-game"].Sessions; got != 1 {
		t.Fatalf("duplicate event counted %d sessions, want 1", got)
	}
}

func TestTelemetryAggregateEndpointsRequireAdminToken(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "")
	srv := newTestServer()
	paths := []string{"/v1/telemetry/perf", "/v1/telemetry/incidents"}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s without configured token: got %d, want 503", path, rec.Code)
		}
	}
	t.Setenv("DQ_TELEMETRY_ADMIN_TOKEN", "telemetry-test-token")
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s without header: got %d, want 403", path, rec.Code)
		}
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Telemetry-Admin-Token", "telemetry-test-token")
		rec = httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s with token: got %d, body %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestTelemetryV2RejectsTrailingJSONValues(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	srv := newTestServer()
	body := `{"schemaVersion":2,"gameId":"game-trailing","runtimeVersion":"test","sessionId":"session-trailing","kind":"session_start","quality":"ok","deviceClass":"test","at":"2026-10-25T00:00:00Z"} {"unexpected":true}`
	if got := postEvent(t, srv, body); got != http.StatusBadRequest {
		t.Fatalf("trailing JSON value got status %d, want %d", got, http.StatusBadRequest)
	}
}

func TestTelemetryV2RejectsTrailingGarbage(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
	srv := newTestServer()
	body := `{"schemaVersion":2,"gameId":"game-trailing","runtimeVersion":"test","sessionId":"session-trailing","kind":"session_start","quality":"ok","deviceClass":"test","at":"2026-10-25T00:00:00Z"} garbage`
	if got := postEvent(t, srv, body); got != http.StatusBadRequest {
		t.Fatalf("trailing garbage got status %d, want %d", got, http.StatusBadRequest)
	}
}
