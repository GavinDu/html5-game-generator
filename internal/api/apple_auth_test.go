package api

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// appleTestEnv serves an Apple-style JWKS from a local server and mints
// identity tokens with the matching private key.
type appleTestEnv struct {
	t      *testing.T
	key    *rsa.PrivateKey
	server *httptest.Server
}

func newAppleTestEnv(t *testing.T) *appleTestEnv {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	env := &appleTestEnv{t: t, key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "test-key-1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	env.server = httptest.NewServer(mux)
	t.Cleanup(env.server.Close)
	t.Setenv("DQ_APPLE_JWKS_URL", env.server.URL)
	t.Setenv("DQ_APPLE_AUDIENCE", "test-audience")
	return env
}

func (env *appleTestEnv) mintIdentity(t *testing.T, mutate func(header map[string]any, claims map[string]any)) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": "test-key-1", "typ": "JWT"}
	claims := map[string]any{
		"iss": "https://appleid.apple.com",
		"aud": "test-audience",
		"exp": time.Now().Add(time.Hour).Unix(),
		"sub": "apple-user-1",
		"iat": time.Now().Unix(),
	}
	if mutate != nil {
		mutate(header, claims)
	}
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := enc(header) + "." + enc(claims)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, env.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestAppleIdentityVerification(t *testing.T) {
	env := newAppleTestEnv(t)
	ctx := t.Context()
	if _, err := verifyAppleIdentityToken(ctx, env.mintIdentity(t, nil)); err != nil {
		t.Fatalf("valid identity token rejected: %v", err)
	}
	cases := map[string]func(map[string]any, map[string]any){
		"wrong audience": func(_ map[string]any, c map[string]any) { c["aud"] = "someone-else" },
		"wrong issuer":   func(_ map[string]any, c map[string]any) { c["iss"] = "https://evil.example" },
		"expired":        func(_ map[string]any, c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"unknown kid":    func(h map[string]any, _ map[string]any) { h["kid"] = "no-such-key" },
		"wrong alg":      func(h map[string]any, _ map[string]any) { h["alg"] = "HS256" },
		"empty sub":      func(_ map[string]any, c map[string]any) { c["sub"] = "" },
	}
	for name, mutate := range cases {
		if _, err := verifyAppleIdentityToken(ctx, env.mintIdentity(t, mutate)); err == nil {
			t.Errorf("%s: identity token accepted, want rejection", name)
		}
	}
	// A tampered payload invalidates the signature.
	good := env.mintIdentity(t, nil)
	parts := bytes.Split([]byte(good), []byte("."))
	parts[1] = []byte(base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://appleid.apple.com","aud":"test-audience","exp":9999999999,"sub":"attacker"}`)))
	tampered := string(parts[0]) + "." + string(parts[1]) + "." + string(parts[2])
	if _, err := verifyAppleIdentityToken(ctx, tampered); err == nil {
		t.Errorf("tampered identity token accepted, want rejection")
	}
}

func TestAppleSignInIssuesUsableSession(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	env := newAppleTestEnv(t)
	srv := newTestServer()

	reqBody, _ := json.Marshal(map[string]string{"identity_token": env.mintIdentity(t, nil)})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/apple", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in got %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Subject     string `json:"subject"`
	}
	decode(t, rec, &issued)
	if issued.TokenType != "Bearer" || issued.ExpiresIn != int(sessionTTL/time.Second) {
		t.Fatalf("unexpected session metadata: %+v", issued)
	}
	if issued.Subject != "apple:apple-user-1" {
		t.Fatalf("subject = %q, want apple:apple-user-1", issued.Subject)
	}

	// The issued session works on a protected route.
	req = httptest.NewRequest(http.MethodPost, "/v1/users/me/preferences/topics", nil)
	req.Header.Set("Authorization", "Bearer "+issued.AccessToken)
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("issued session rejected: %d %s", rec.Code, rec.Body.String())
	}

	// An invalid Apple token gets 401, not a session.
	reqBody, _ = json.Marshal(map[string]string{"identity_token": "not.a.token"})
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/apple", bytes.NewReader(reqBody))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid identity token got %d, want 401", rec.Code)
	}
}

func TestAppleSignInFailsClosedWithoutSecret(t *testing.T) {
	t.Setenv("DQ_AUTH_HMAC_SECRET", "")
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	env := newAppleTestEnv(t)
	srv := newTestServer()
	reqBody, _ := json.Marshal(map[string]string{"identity_token": env.mintIdentity(t, nil)})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/apple", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured sign-in got %d, want 503", rec.Code)
	}
}

func TestDeleteAccountPurgesDataAndInvalidatesSession(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	env := newAppleTestEnv(t)
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "delete-me-game")

	// Sign in, then build social state as that user.
	reqBody, _ := json.Marshal(map[string]string{"identity_token": env.mintIdentity(t, nil)})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/apple", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sign-in got %d: %s", rec.Code, rec.Body.String())
	}
	var issued struct {
		AccessToken string `json:"access_token"`
		Subject     string `json:"subject"`
	}
	decode(t, rec, &issued)
	auth := func(method, path string, body any) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+issued.AccessToken)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	if r := auth(http.MethodPost, "/v1/users/parent-1/follow", nil); r.Code != http.StatusOK {
		t.Fatalf("follow got %d: %s", r.Code, r.Body.String())
	}
	if r := auth(http.MethodPost, "/v1/games/delete-me-game/save", nil); r.Code != http.StatusOK {
		t.Fatalf("save got %d: %s", r.Code, r.Body.String())
	}
	if r := auth(http.MethodPost, "/v1/games/delete-me-game/reactions", map[string]string{"reaction": "like"}); r.Code != http.StatusOK {
		t.Fatalf("react got %d: %s", r.Code, r.Body.String())
	}
	if r := auth(http.MethodPost, "/v1/games/delete-me-game/comments", map[string]string{"body": "hello"}); r.Code != http.StatusCreated {
		t.Fatalf("comment got %d: %s", r.Code, r.Body.String())
	}
	_ = game

	// Delete the account.
	if r := auth(http.MethodDelete, "/v1/users/me", nil); r.Code != http.StatusOK {
		t.Fatalf("delete account got %d: %s", r.Code, r.Body.String())
	}
	srv.socialMu.RLock()
	_, stillFollowing := srv.follows[issued.Subject]
	_, stillSaved := srv.saves[issued.Subject]
	_, stillReacted := srv.reactions[issued.Subject]
	commentsLeft := len(srv.comments["delete-me-game"])
	srv.socialMu.RUnlock()
	if stillFollowing || stillSaved || stillReacted || commentsLeft != 0 {
		t.Fatalf("social data remains after deletion: follows=%v saves=%v reactions=%v comments=%d",
			stillFollowing, stillSaved, stillReacted, commentsLeft)
	}

	// The old session no longer works on protected routes.
	if r := auth(http.MethodPost, "/v1/users/me/preferences/topics", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("deleted subject session got %d, want 401", r.Code)
	}
	// And the same Apple identity cannot sign in again.
	reqBody, _ = json.Marshal(map[string]string{"identity_token": env.mintIdentity(t, nil)})
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/apple", bytes.NewReader(reqBody))
	rec = httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("deleted subject re-issue got %d, want 403", rec.Code)
	}
}

func TestCrossUserCommentDeleteForbidden(t *testing.T) {
	const secret = "test-only-secret-that-is-long-enough"
	t.Setenv("DQ_AUTH_HMAC_SECRET", secret)
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "false")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	seedReadyGame(st, "cross-user-game")
	tokenA := authTokenForTest("user-a", time.Now().Add(time.Hour), secret)
	tokenB := authTokenForTest("user-b", time.Now().Add(time.Hour), secret)
	call := func(token, method, path string, body any) *httptest.ResponseRecorder {
		var rdr *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rdr = bytes.NewReader(b)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, rdr)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}
	// B authors a comment.
	r := call(tokenB, http.MethodPost, "/v1/games/cross-user-game/comments", map[string]string{"body": "mine"})
	if r.Code != http.StatusCreated {
		t.Fatalf("B comment got %d: %s", r.Code, r.Body.String())
	}
	// Read the created comment ID from server state.
	srv.socialMu.RLock()
	comments := srv.comments["cross-user-game"]
	srv.socialMu.RUnlock()
	if len(comments) != 1 {
		t.Fatalf("want 1 comment, got %d", len(comments))
	}
	// A cannot delete B's comment.
	if r := call(tokenA, http.MethodDelete, "/v1/games/cross-user-game/comments/"+comments[0].ID, nil); r.Code != http.StatusForbidden {
		t.Fatalf("cross-user delete got %d, want 403", r.Code)
	}
	// B can delete their own.
	if r := call(tokenB, http.MethodDelete, "/v1/games/cross-user-game/comments/"+comments[0].ID, nil); r.Code != http.StatusOK {
		t.Fatalf("owner delete got %d, want 200: %s", r.Code, r.Body.String())
	}
}
