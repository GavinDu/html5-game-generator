package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestMyProjectsUsesVerifiedOwnerAndStableCursor(t *testing.T) {
	const secret = "arcade-pulse-test-secret-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))

	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	now := time.Date(2026, time.November, 10, 10, 0, 0, 0, time.UTC)
	seed := func(id, owner, title string, status model.GameStatus, created time.Time) {
		t.Helper()
		st.CreateGame(&model.Game{
			ID: id, OwnerID: owner, Title: title, Status: status,
			Visibility: model.VisibilityPrivate, CreatedAt: created, UpdatedAt: created,
		})
	}
	seed("alpha-new", "owner-a", "secret-draft", model.GameDraft, now)
	seed("alpha-old", "owner-a", "ready-game", model.GameReady, now.Add(-time.Hour))
	seed("beta-secret", "owner-b", "other-users-private-game", model.GameDraft, now.Add(-2*time.Minute))
	st.CreateJob(&model.Job{ID: "job-alpha-new", GameID: "alpha-new", Status: model.JobRunning})
	get := func(subject, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+
			authTokenForTest(subject, time.Now().Add(time.Hour), secret))
		// A caller-controlled spoofed header must never affect ownership.
		req.Header.Set("X-User-ID", "owner-b")
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	first := get("owner-a", "/v1/users/me/projects?limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first projects status = %d, body = %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("projects cache policy = %q", got)
	}
	var page ownerProjectsResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Projects) != 1 || page.Projects[0].GameID != "alpha-new" ||
		page.Projects[0].JobStatus != model.JobRunning || page.NextCursor == nil {
		t.Fatalf("unexpected first owner projects page: %#v", page)
	}
	if strings.Contains(first.Body.String(), "other-users-private-game") ||
		strings.Contains(first.Body.String(), "elements") {
		t.Fatalf("owner projects leaked private or full model data: %s", first.Body.String())
	}

	second := get("owner-a", "/v1/users/me/projects?limit=1&cursor="+*page.NextCursor)
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d: %s", second.Code, second.Body.String())
	}
	var next ownerProjectsResponse
	if err := json.Unmarshal(second.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Projects) != 1 || next.Projects[0].GameID != "alpha-old" ||
		next.NextCursor != nil {
		t.Fatalf("owner cursor page = %#v", next)
	}

	ready := get("owner-a", "/v1/users/me/projects?status=ready")
	var filtered ownerProjectsResponse
	if err := json.Unmarshal(ready.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if ready.Code != http.StatusOK || len(filtered.Projects) != 1 ||
		filtered.Projects[0].GameID != "alpha-old" {
		t.Fatalf("ready filter = %d %#v", ready.Code, filtered)
	}

	other := get("owner-b", "/v1/users/me/projects")
	if other.Code != http.StatusOK || strings.Contains(other.Body.String(), "alpha-new") ||
		!strings.Contains(other.Body.String(), "beta-secret") {
		t.Fatalf("cross-owner private list leak or missing owner record: %d %s",
			other.Code, other.Body.String())
	}
}

func TestMyProjectsRejectsUnauthenticatedAndInvalidPagination(t *testing.T) {
	const secret = "arcade-pulse-test-secret-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))

	unauthenticated := httptest.NewRequest(http.MethodGet, "/v1/users/me/projects", nil)
	unauthenticated.Header.Set("X-User-ID", "impersonated")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, unauthenticated)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed identity got %d, want 401: %s", rec.Code, rec.Body.String())
	}

	for _, path := range []string{
		"/v1/users/me/projects?limit=-1",
		"/v1/users/me/projects?limit=51",
		"/v1/users/me/projects?limit=abc",
		"/v1/users/me/projects?status=private",
		"/v1/users/me/projects?cursor=not-base64***",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+
			authTokenForTest("verified-owner", time.Now().Add(time.Hour), secret))
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("invalid input %s status = %d, want 400: %s", path, rec.Code, rec.Body.String())
		}
	}
}
