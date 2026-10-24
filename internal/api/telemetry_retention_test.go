package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneTelemetryFilesKeepsRecentAndUndatedRecords(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "moderation-audit.jsonl")
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	t.Setenv("DQ_MODERATION_AUDIT_FILE", auditPath)

	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)

	files := map[string]string{
		filepath.Join(dir, "events.jsonl"): strings.Join([]string{
			`{"receivedAt":"` + old + `","event":{"kind":"session_start"}}`,
			`{"receivedAt":"` + recent + `","event":{"kind":"run_end"}}`,
			"not-json-but-preserve",
		}, "\n") + "\n",
		filepath.Join(dir, "runtime-errors.jsonl"): strings.Join([]string{
			`{"receivedAt":"` + old + `","fingerprint":"old"}`,
			`{"receivedAt":"` + recent + `","fingerprint":"recent"}`,
		}, "\n") + "\n",
		auditPath: strings.Join([]string{
			`{"created_at":"` + old + `","action":"old"}`,
			`{"created_at":"` + recent + `","action":"recent"}`,
		}, "\n") + "\n",
	}
	for path, data := range files {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneTelemetryFiles(dir, 30, now); err != nil {
		t.Fatalf("prune telemetry: %v", err)
	}
	for path, want := range map[string][]string{
		filepath.Join(dir, "events.jsonl"): {"run_end", "not-json-but-preserve"},
		filepath.Join(dir, "runtime-errors.jsonl"): {"recent"},
		auditPath: {"recent"},
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Contains(text, `"old"`) || strings.Contains(text, old) {
			t.Errorf("%s retained expired data: %s", path, text)
		}
		for _, token := range want {
			if !strings.Contains(text, token) {
				t.Errorf("%s lost expected record %q: %s", path, token, text)
			}
		}
	}
}

func TestTelemetryRetentionIsOptInAndRejectsInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_TELEMETRY_DIR", dir)
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(dir, "moderation-audit.jsonl"))
	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	path := filepath.Join(dir, "events.jsonl")
	record := `{"receivedAt":"` + old + `","event":{"kind":"session_start"}}` + "\n"
	if err := os.WriteFile(path, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DQ_TELEMETRY_RETENTION_DAYS", "")
	if err := pruneTelemetryOnStartup(); err != nil {
		t.Fatalf("unset retention should be disabled: %v", err)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != record {
		t.Fatalf("unset retention changed the file: err=%v content=%q", err, raw)
	}

	t.Setenv("DQ_TELEMETRY_RETENTION_DAYS", "0")
	if err := pruneTelemetryOnStartup(); err == nil {
		t.Fatal("expected invalid retention configuration to be rejected")
	}
}
