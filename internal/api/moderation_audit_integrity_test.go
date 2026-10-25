package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
