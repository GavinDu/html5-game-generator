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
