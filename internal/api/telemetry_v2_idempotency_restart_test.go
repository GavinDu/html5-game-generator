package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestTelemetryV2IdempotencySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	st := store.NewMemoryStore()
	first := New(st, pipeline.NewRunner(st))
	body := `{"schemaVersion":2,"eventId":"stable-event","gameId":"game-a","sessionId":"session-a","kind":"session_start","at":"2026-10-25T00:00:00Z"}`
	if code := postEvent(t, first, body); code != http.StatusAccepted {
		t.Fatalf("first post got %d", code)
	}
	logPath := filepath.Join(dir, "events.jsonl")
	before, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a new server process over the same durable event log.
	second := New(st, pipeline.NewRunner(st))
	if code := postEvent(t, second, body); code != http.StatusAccepted {
		t.Fatalf("retry after restart got %d", code)
	}
	after, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("duplicate event appended after restart: before %d bytes, after %d bytes", len(before), len(after))
	}

	// A distinct event must still be accepted and persisted.
	distinct := `{"schemaVersion":2,"eventId":"stable-event-2","gameId":"game-a","sessionId":"session-a","kind":"session_start","at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	if code := postEvent(t, second, distinct); code != http.StatusAccepted {
		t.Fatalf("distinct post got %d", code)
	}
	afterDistinct, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterDistinct) <= len(after) {
		t.Fatal("distinct event was not persisted")
	}
}

func TestTelemetryV2MalformedLogDoesNotBlockValidRetryIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	logPath := filepath.Join(dir, "events.jsonl")
	valid := `{"receivedAt":"2026-10-25T00:00:00Z","event":{"schemaVersion":2,"eventId":"persisted-event","gameId":"game-a","sessionId":"session-a","kind":"session_start","at":"2026-10-25T00:00:00Z"}}`
	if err := os.WriteFile(logPath, []byte(valid+"\nnot-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := New(store.NewMemoryStore(), pipeline.NewRunner(store.NewMemoryStore()))
	body := `{"schemaVersion":2,"eventId":"persisted-event","gameId":"game-a","sessionId":"session-a","kind":"session_start","at":"2026-10-25T00:00:00Z"}`
	if code := postEvent(t, srv, body); code != http.StatusAccepted {
		t.Fatalf("retry against existing event got %d, want %d", code, http.StatusAccepted)
	}
	after, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(after), `"eventId":"persisted-event"`) != 1 {
		t.Fatalf("existing event was duplicated: %s", after)
	}
}

func TestTelemetryV2SummariesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	st := store.NewMemoryStore()
	first := New(st, pipeline.NewRunner(st))
	events := []string{
		`{"schemaVersion":2,"eventId":"session-1","gameId":"game-a","sessionId":"session-a","kind":"session_start","at":"2026-10-25T00:00:00Z"}`,
		`{"schemaVersion":2,"eventId":"perf-1","gameId":"game-a","sessionId":"session-a","kind":"perf_window","frames":100,"buckets":[10,20,30,20,10,10],"at":"2026-10-25T00:00:05Z"}`,
		`{"schemaVersion":2,"eventId":"run-1","gameId":"game-a","sessionId":"session-a","kind":"run_end","won":true,"durationMs":12000,"inputs":10,"deadInputs":2,"at":"2026-10-25T00:00:10Z"}`,
		`{"schemaVersion":2,"eventId":"mile-1","gameId":"game-a","sessionId":"session-a","kind":"milestone","milestone":"first_input","ms":1500,"at":"2026-10-25T00:00:02Z"}`,
	}
	for _, body := range events {
		if code := postEvent(t, first, body); code != http.StatusAccepted {
			t.Fatalf("event post got %d, want %d: %s", code, http.StatusAccepted, body)
		}
	}
	before := first.perfSummaries()["game-a"]
	if before == nil || before.Sessions != 1 || before.Runs != 1 || before.PerfWindows != 1 || before.Frames != 100 || before.FirstInputMedianMs != 1500 {
		t.Fatalf("unexpected pre-restart summary: %+v", before)
	}

	second := New(st, pipeline.NewRunner(st))
	after := second.perfSummaries()["game-a"]
	if after == nil {
		t.Fatal("telemetry summary disappeared after restart")
	}
	if after.Sessions != before.Sessions || after.Runs != before.Runs ||
		after.WinRate != before.WinRate || after.AvgRunMs != before.AvgRunMs ||
		after.PerfWindows != before.PerfWindows || after.Frames != before.Frames ||
		after.JankRate != before.JankRate || after.P95FrameMs != before.P95FrameMs ||
		after.FirstInputMedianMs != before.FirstInputMedianMs {
		t.Fatalf("summary changed after restart: before=%+v after=%+v", before, after)
	}
}
