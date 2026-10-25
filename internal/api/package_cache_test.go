package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func packageRequest(t *testing.T, srv *Server, etag, user string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/games/cache-policy-game/package", nil)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func TestPublicPackageETagAndCacheInvalidation(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "cache-policy-game")
	game.ModerationStatus = "approved"
	st.UpdateGame(game)

	first := packageRequest(t, srv, "", "")
	if first.Code != http.StatusOK {
		t.Fatalf("initial package: got %d: %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("public package response missing ETag")
	}
	if got := first.Header().Get("Cache-Control"); got != "public, max-age=300, must-revalidate" {
		t.Fatalf("public Cache-Control = %q", got)
	}

	cached := packageRequest(t, srv, etag, "")
	if cached.Code != http.StatusNotModified {
		t.Fatalf("conditional package request: got %d, want 304", cached.Code)
	}
	if cached.Body.Len() != 0 {
		t.Fatalf("304 response unexpectedly has body: %q", cached.Body.String())
	}

	// A published content change must produce a new strong ETag so a
	// previously cached package is not reused for different IR content.
	updated, ok := st.GetGame("cache-policy-game")
	if !ok || updated.GameIR == nil {
		t.Fatal("seeded game has no Game IR")
	}
	updated.GameIR.Game.Title += " changed"
	st.UpdateGame(updated)

	after := packageRequest(t, srv, etag, "")
	if after.Code != http.StatusOK {
		t.Fatalf("changed package with old ETag: got %d, want 200", after.Code)
	}
	if got := after.Header().Get("ETag"); got == etag {
		t.Fatalf("ETag did not change after package content changed: %q", got)
	}
}

func TestRestrictedPackageIsNeverSharedCached(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "cache-policy-game")
	game.Visibility = model.VisibilityPrivate
	st.UpdateGame(game)

	first := packageRequest(t, srv, "", "parent-1")
	if first.Code != http.StatusOK {
		t.Fatalf("owner preview: got %d: %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("restricted package response missing ETag")
	}
	if got := first.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("restricted Cache-Control = %q", got)
	}
	if got := first.Header().Get("Vary"); got != "X-User-ID" {
		t.Fatalf("restricted Vary = %q", got)
	}

	// A matching conditional header must not turn owner-only content into
	// a shared 304 response.
	again := packageRequest(t, srv, etag, "parent-1")
	if again.Code != http.StatusOK {
		t.Fatalf("restricted conditional request: got %d, want 200", again.Code)
	}
}
