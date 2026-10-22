package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestTelemetryEventsAcceptedAndSummarized(t *testing.T) {
	t.Setenv("DQ_TELEMETRY_DIR", t.TempDir())
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
