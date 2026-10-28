package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The web runtime (pixi-runtime.js) reports the Hephaestus #61 timing
// fields itself, because the bridgeless iOS shell cannot observe them:
// player_ready at boot carries time to interactive, and
// player_session_summary at run end carries the full frame summary.
// These tests post the exact payloads the runtime sends and prove they
// land on stored social events end to end, with retries deduplicated.

func readSocialEventsJSONL(t *testing.T, dir string) []socialEvent {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "social-events.jsonl"))
	if err != nil {
		t.Fatalf("open persisted events: %v", err)
	}
	defer f.Close()
	var out []socialEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var ev socialEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("decode persisted event: %v", err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read persisted events: %v", err)
	}
	return out
}

func TestRuntimePlayerSessionSummaryIsStoredAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	// Exact payload shape the runtime sends at run end.
	body := map[string]any{
		"schema_version":         1,
		"event_id":               "abcdef01:run0:summary",
		"game_id":                "game-1",
		"kind":                   "player_session_summary",
		"time_to_interactive_ms": 450,
		"time_to_first_input_ms": 800,
		"frame_time_p95_ms":      25,
		"frame_time_avg_ms":      17,
		"frame_count":            300,
	}
	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept runtime summary: %d %s", resp.Code, resp.Body.String())
	}

	// A retry with the same event ID but different measurements must
	// neither overwrite the original nor append a second persisted event.
	body["time_to_interactive_ms"] = 9999
	body["frame_time_p95_ms"] = 999
	resp = do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("ack duplicate summary: %d %s", resp.Code, resp.Body.String())
	}
	persisted := readSocialEventsJSONL(t, dir)
	if len(persisted) != 1 {
		t.Fatalf("persisted %d events, want exactly one", len(persisted))
	}
	got := persisted[0]
	if got.Kind != "player_session_summary" || got.EventID != "abcdef01:run0:summary" ||
		got.TimeToInteractiveMS != 450 || got.TimeToFirstInputMS != 800 ||
		got.FrameTimeP95MS != 25 || got.FrameTimeAvgMS != 17 || got.FrameCount != 300 {
		t.Fatalf("persisted summary did not match first delivery: %+v", got)
	}

	// A later run's summary has a different event ID and is stored separately.
	body["event_id"] = "abcdef01:run1:summary"
	body["time_to_interactive_ms"] = 500
	resp = do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept second run summary: %d %s", resp.Code, resp.Body.String())
	}
	if len(readSocialEventsJSONL(t, dir)) != 2 {
		t.Fatalf("want 2 persisted events after second run, got %d", len(readSocialEventsJSONL(t, dir)))
	}
}

func TestRuntimePlayerReadyCarriesTimeToInteractive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	// Exact payload shape the runtime sends at boot.
	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", map[string]any{
		"schema_version":         1,
		"event_id":               "abcdef01:run0:ready",
		"game_id":                "game-1",
		"kind":                   "player_ready",
		"time_to_interactive_ms": 450,
	}, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept runtime player_ready: %d %s", resp.Code, resp.Body.String())
	}
	persisted := readSocialEventsJSONL(t, dir)
	if len(persisted) != 1 || persisted[0].Kind != "player_ready" ||
		persisted[0].TimeToInteractiveMS != 450 {
		t.Fatalf("persisted player_ready mismatch: %+v", persisted)
	}
}

func TestRuntimePlayerDiagnosticEventsAreAcceptedAnonymous(t *testing.T) {
	// The runtime posts with apiFetch, which attaches a bearer token only
	// when the tab holds one; unsigned web players arrive anonymous. The
	// diagnostic kinds must still be accepted and stored.
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", map[string]any{
		"schema_version":         1,
		"event_id":               "abcdef01:run0:summary",
		"game_id":                "game-1",
		"kind":                   "player_session_summary",
		"time_to_interactive_ms": 450,
		"time_to_first_input_ms": 800,
		"frame_time_p95_ms":      25,
		"frame_time_avg_ms":      17,
		"frame_count":            300,
	}, "")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept anonymous runtime summary: %d %s", resp.Code, resp.Body.String())
	}
	persisted := readSocialEventsJSONL(t, dir)
	if len(persisted) != 1 || persisted[0].FrameCount != 300 {
		t.Fatalf("persisted anonymous summary mismatch: %+v", persisted)
	}
}

func TestRuntimePlayerDiagnosticEventsRejectOutOfRangeMeasurements(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"summary p95 over cap", map[string]any{"schema_version": 1, "event_id": "s1", "game_id": "game-1", "kind": "player_session_summary", "frame_time_p95_ms": 60001}},
		{"summary avg over cap", map[string]any{"schema_version": 1, "event_id": "s2", "game_id": "game-1", "kind": "player_session_summary", "frame_time_avg_ms": 60001}},
		{"summary negative first input", map[string]any{"schema_version": 1, "event_id": "s3", "game_id": "game-1", "kind": "player_session_summary", "time_to_first_input_ms": -1}},
		{"ready over cap", map[string]any{"schema_version": 1, "event_id": "r1", "game_id": "game-1", "kind": "player_ready", "time_to_interactive_ms": 3600001}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", tc.body, "viewer")
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want 400: %s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestPixiRuntimeEmitsPlayerDiagnosticSocialEvents(t *testing.T) {
	// Guard the wiring: the runtime file the server embeds must contain
	// the diagnostic emission, or the backend acceptance above has no
	// sender in production.
	for _, want := range []string{
		"/v1/telemetry/social-event",
		"player_session_summary",
		"player_ready",
		"time_to_interactive_ms",
		"time_to_first_input_ms",
		"frame_time_p95_ms",
		"frame_time_avg_ms",
		"frame_count",
		"frameP95Ms",
	} {
		if !strings.Contains(pixiRuntimeJS, want) {
			t.Fatalf("embedded pixi-runtime.js is missing %q", want)
		}
	}
}
