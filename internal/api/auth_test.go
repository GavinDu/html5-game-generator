package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"\n\n\t"doodlequest/internal/model"\n\t"doodlequest/internal/pipeline"\n\t"doodlequest/internal/store"
	"time"
)

func TestUserAuthFailsClosedWithoutConfiguration(t *testing.T) {
	t.Setenv("DQ_AUTH_HMAC_SECRET", "")
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/v1/users/me/preferences/topics", nil)
	req.Header.Set("X-User-ID", "spoofed-user")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured auth got %d, want 503: %s", rec.Code, rec.Body.String())
	}
}

func TestUserAuthRejectsSpoofedIdentityAndAcceptsSignedBearer(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	srv := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/v1/users/me/preferences/topics", nil)
	req.Header.Set("X-User-ID", "spoofed-user")
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("spoofed identity got %d, want 401: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/users/me/preferences/topics", nil)
	req.Header.Set("Authorization", "Bearer "+authTokenForTest("verified-user", time.Now().Add(time.Hour), secret))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("valid bearer rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUserAuthRejectsExpiredAndTamperedTokens(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	srv := newTestServer()
	for _, token := range []string{
		authTokenForTest("user-a", time.Now().Add(-time.Minute), secret),
		authTokenForTest("user-a", time.Now().Add(time.Hour), secret) + "tampered",
		"not-a-token",
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/users/me/preferences/topics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q got %d, want 401", token, rec.Code)
		}
	}
}

func TestPrivateDeliveryUsesVerifiedIdentityNotXUserID(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "private-auth-game")
	game.Visibility = model.VisibilityPrivate
	st.UpdateGame(game)

	// A caller-controlled identity header must not grant access to a private package.
	req := httptest.NewRequest(http.MethodGet, "/v1/games/private-auth-game/package", nil)
	req.Header.Set("X-User-ID", game.OwnerID)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("spoofed X-User-ID got %d, want 404: %s", rec.Code, rec.Body.String())
	}

	// The same owner is allowed when their identity comes from a verified bearer.
	req = httptest.NewRequest(http.MethodGet, "/v1/games/private-auth-game/package", nil)
	req.Header.Set("Authorization", "Bearer "+authTokenForTest(game.OwnerID, time.Now().Add(time.Hour), secret))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verified owner got %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
