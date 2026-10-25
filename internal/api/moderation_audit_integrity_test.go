package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModerationAuditFailsClosedOnMalformedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte("{\"id\":\"good\",\"action\":\"approved\"}\nnot-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DQ_MODERATION_AUDIT_FILE", path)
	t.Setenv("DQ_MODERATION_TOKEN", "moderation-test-token")

	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/moderation/audit", nil)
	req.Header.Set("X-Moderation-Token", "moderation-test-token")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestModerationAuditReturnsEmptyListWhenFileMissing(t *testing.T) {
	t.Setenv("DQ_MODERATION_AUDIT_FILE", filepath.Join(t.TempDir(), "missing.jsonl"))
	t.Setenv("DQ_MODERATION_TOKEN", "moderation-test-token")

	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/v1/moderation/audit", nil)
	req.Header.Set("X-Moderation-Token", "moderation-test-token")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}


func TestPersistModerationAuditAppendsDurableJSONLRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("DQ_MODERATION_AUDIT_FILE", path)
	events := []moderationAuditEvent{
		{ID: "event-1", Action: "approve", GameID: "game-1"},
		{ID: "event-2", Action: "quarantine", GameID: "game-2"},
	}
	for _, event := range events {
		if err := persistModerationAudit(event); err != nil {
			t.Fatalf("persist %s: %v", event.ID, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(events) {
		t.Fatalf("record count = %d, want %d; contents=%q", len(lines), len(events), data)
	}
	for i, line := range lines {
		var got moderationAuditEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("record %d is invalid JSON: %v", i, err)
		}
		if got.ID != events[i].ID || got.Action != events[i].Action {
			t.Fatalf("record %d = %+v, want %+v", i, got, events[i])
		}
	}
}
