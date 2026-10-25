package api

import (
	"net/http"
	"path/filepath"
	"testing"
)

func TestSocialEventPerformanceFieldsAreVersionedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	body := map[string]any{
		"schema_version":         1,
		"event_id":               "event-1",
		"game_id":                "game-1",
		"kind":                   "play_start",
		"preview_dwell_ms":       1200,
		"time_to_interactive_ms": 450,
		"time_to_first_input_ms": 800,
		"frame_time_p95_ms":      24,
		"frame_time_avg_ms":      17,
		"frame_count":            300,
	}
	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept performance event: %d %s", resp.Code, resp.Body.String())
	}

	// A retry with the same event ID must not overwrite the original measurements.
	body["preview_dwell_ms"] = 9999
	resp = do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("ack duplicate event: %d %s", resp.Code, resp.Body.String())
	}

	srv.socialMu.Lock()
	defer srv.socialMu.Unlock()
	if len(srv.events) != 1 {
		t.Fatalf("stored %d events, want exactly one", len(srv.events))
	}
	got := srv.events[0]
	if got.SchemaVersion != 1 || got.PreviewDwellMS != 1200 ||
		got.TimeToInteractiveMS != 450 || got.TimeToFirstInputMS != 800 ||
		got.FrameTimeP95MS != 24 || got.FrameTimeAvgMS != 17 || got.FrameCount != 300 {
		t.Fatalf("stored measurements did not match first event: %+v", got)
	}
}

func TestSocialEventPerformanceFieldsRejectInvalidMeasurementsAndSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"unsupported schema", map[string]any{"schema_version": 2, "game_id": "game-1", "kind": "impression"}},
		{"negative dwell", map[string]any{"schema_version": 1, "game_id": "game-1", "kind": "impression", "preview_dwell_ms": -1}},
		{"excessive interactive time", map[string]any{"schema_version": 1, "game_id": "game-1", "kind": "play_start", "time_to_interactive_ms": 3600001}},
		{"excessive frame time", map[string]any{"schema_version": 1, "game_id": "game-1", "kind": "play_start", "frame_time_p95_ms": 60001}},
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

func TestSocialEventLegacyClientDefaultsToSchemaV1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", map[string]any{
		"event_id": "legacy-1", "game_id": "game-1", "kind": "impression",
	}, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("legacy event should remain accepted: %d %s", resp.Code, resp.Body.String())
	}
	srv.socialMu.Lock()
	defer srv.socialMu.Unlock()
	if len(srv.events) != 1 || srv.events[0].SchemaVersion != 1 {
		t.Fatalf("legacy event should be stored as schema v1: %+v", srv.events)
	}
}

func TestSocialPreviewDwellEventIsAcceptedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "game-1", "creator")

	body := map[string]any{
		"schema_version":       1,
		"event_id":             "preview-dwell-1",
		"game_id":              "game-1",
		"kind":                 "preview_dwell",
		"preview_dwell_ms":     700,
		"duration_ms":          700,
		"expected_duration_ms": 700,
	}
	resp := do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("accept preview dwell event: %d %s", resp.Code, resp.Body.String())
	}
	body["preview_dwell_ms"] = 1200
	resp = do(t, srv, http.MethodPost, "/v1/telemetry/social-event", body, "viewer")
	if resp.Code != http.StatusAccepted {
		t.Fatalf("ack duplicate preview dwell event: %d %s", resp.Code, resp.Body.String())
	}

	srv.socialMu.Lock()
	defer srv.socialMu.Unlock()
	if len(srv.events) != 1 {
		t.Fatalf("stored %d events, want exactly one", len(srv.events))
	}
	if srv.events[0].Kind != "preview_dwell" || srv.events[0].PreviewDwellMS != 700 {
		t.Fatalf("unexpected stored preview dwell event: %+v", srv.events[0])
	}
}
