package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestModerationReportQueueFiltersAndLimit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social.json"))
	t.Setenv("DQ_MODERATION_TOKEN", "queue-test-token")
	srv := newTestServer()
	now := time.Now().UTC()
	srv.socialMu.Lock()
	srv.reports = []socialReport{
		{ID: "r-old", GameID: "game-a", Reporter: "u1", Reason: "spam", CreatedAt: now.Add(-3 * time.Hour), Status: "open"},
		{ID: "r-new", GameID: "game-a", Reporter: "u2", Reason: "abuse", CreatedAt: now.Add(-time.Hour), Status: "open"},
		{ID: "r-resolved", GameID: "game-b", Reporter: "u3", Reason: "other", CreatedAt: now, Status: "resolved"},
	}
	srv.socialMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/moderation/reports?status=open&game_id=game-a&limit=1", nil)
	req.Header.Set("X-Moderation-Token", "queue-test-token")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("queue got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "r-new") || strings.Contains(body, "r-old") || strings.Contains(body, "r-resolved") {
		t.Fatalf("filters/limit returned unexpected reports: %s", body)
	}
}

func TestModerationReportQueueRejectsInvalidFilters(t *testing.T) {
	t.Setenv("DQ_MODERATION_TOKEN", "queue-test-token")
	srv := newTestServer()
	for _, path := range []string{
		"/v1/moderation/reports?status=deleted",
		"/v1/moderation/reports?limit=0",
		"/v1/moderation/reports?limit=501",
		"/v1/moderation/reports?limit=abc",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Moderation-Token", "queue-test-token")
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s got %d, want 400: %s", path, rec.Code, rec.Body.String())
		}
	}
}
