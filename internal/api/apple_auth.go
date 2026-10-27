package api

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Sign in with Apple session issuance (Hephaestus #10).
//
// The backend never trusts a caller-supplied identity. The iOS app proves
// the user with an Apple identity token (RS256 JWT from Apple), this
// package verifies it against Apple's published keys, and only then mints
// the backend's own short-lived HMAC session token (same format the
// verifier already accepts: base64url JSON claims + HMAC-SHA256).
//
// Session policy (see docs/AUTH.md):
//   - Sessions last 30 days and are not refreshed; the user signs in
//     again with Apple when one expires.
//   - There is no server-side session table to revoke: revocation is
//     account deletion (tombstone, below) plus rotating
//     DQ_AUTH_HMAC_SECRET, which invalidates every outstanding session.
//   - Apple-side revocation (Settings > Apple ID > Sign in with Apple)
//     stops future issuance but does not invalidate outstanding backend
//     sessions before their 30-day expiry; that limit is documented,
//     not hidden.

const (
	// sessionTTL is the lifetime of a backend session minted from an
	// Apple identity token.
	sessionTTL = 30 * 24 * time.Hour
	// appleIssuer is the only accepted iss for Apple identity tokens.
	appleIssuer = "https://appleid.apple.com"
	// defaultAppleJWKSURL is Apple's published key set.
	defaultAppleJWKSURL = "https://appleid.apple.com/auth/keys"
	// defaultAppleAudience is the Hephaestus iOS bundle ID. Override with
	// DQ_APPLE_AUDIENCE when the app ships under a different identifier.
	defaultAppleAudience = "ai.tinydream.Hephaestus"
	// appleClockSkew tolerates small clock differences when checking exp.
	appleClockSkew = 5 * time.Minute
)

func appleJWKSURL() string {
	if v := strings.TrimSpace(os.Getenv("DQ_APPLE_JWKS_URL")); v != "" {
		return v
	}
	return defaultAppleJWKSURL
}

func appleAudience() string {
	if v := strings.TrimSpace(os.Getenv("DQ_APPLE_AUDIENCE")); v != "" {
		return v
	}
	return defaultAppleAudience
}

func authSecret() string {
	return strings.TrimSpace(os.Getenv("DQ_AUTH_HMAC_SECRET"))
}

// mintSessionToken signs a backend session token with the same contract
// as verifyBearerUserID. authTokenForTest delegates here so tests and
// production cannot drift apart.
func mintSessionToken(subject string, expires time.Time, secret string) string {
	payload, _ := json.Marshal(authClaims{Subject: subject, Expires: expires.Unix(), Issuer: "hephaestus"})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type appleJWKS struct {
	Keys []appleJWK `json:"keys"`
}

type appleJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type appleClaims struct {
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
	Subject  string `json:"sub"`
}

var (
	jwksMu     sync.Mutex
	jwksCached *appleJWKS
	jwksURL    string
	jwksAt     time.Time
)

// fetchAppleJWKS returns Apple's key set, cached for one hour per URL.
// Tests point DQ_APPLE_JWKS_URL at a local server; the URL is part of
// the cache key so a test server never serves production keys or vice
// versa.
func fetchAppleJWKS(ctx context.Context) (*appleJWKS, error) {
	url := appleJWKSURL()
	jwksMu.Lock()
	defer jwksMu.Unlock()
	if jwksCached != nil && jwksURL == url && time.Since(jwksAt) < time.Hour {
		return jwksCached, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Apple JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch Apple JWKS: status %d", resp.StatusCode)
	}
	var set appleJWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode Apple JWKS: %w", err)
	}
	jwksCached = &set
	jwksURL = url
	jwksAt = time.Now()
	return &set, nil
}

// verifyAppleIdentityToken checks an Apple identity token and returns the
// Apple subject (opaque user ID) on success.
func verifyAppleIdentityToken(ctx context.Context, raw string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("identity token must have three segments")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode identity header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return "", fmt.Errorf("decode identity header: %w", err)
	}
	if header.Alg != "RS256" {
		return "", fmt.Errorf("unexpected identity alg %q", header.Alg)
	}
	if strings.TrimSpace(header.Kid) == "" {
		return "", errors.New("identity token has no kid")
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode identity payload: %w", err)
	}
	var claims appleClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return "", fmt.Errorf("decode identity payload: %w", err)
	}
	if claims.Issuer != appleIssuer {
		return "", fmt.Errorf("unexpected identity iss %q", claims.Issuer)
	}
	if claims.Audience != appleAudience() {
		return "", fmt.Errorf("unexpected identity aud %q", claims.Audience)
	}
	if claims.Subject == "" || len(claims.Subject) > 128 {
		return "", errors.New("identity sub must be 1–128 characters")
	}
	if time.Unix(claims.Expires, 0).Add(appleClockSkew).Before(time.Now()) {
		return "", errors.New("identity token is expired")
	}
	set, err := fetchAppleJWKS(ctx)
	if err != nil {
		return "", err
	}
	var key *appleJWK
	for i := range set.Keys {
		if set.Keys[i].Kid == header.Kid {
			key = &set.Keys[i]
			break
		}
	}
	if key == nil {
		return "", fmt.Errorf("no Apple key for kid %q", header.Kid)
	}
	if key.Kty != "RSA" {
		return "", fmt.Errorf("unexpected Apple key type %q", key.Kty)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return "", fmt.Errorf("decode Apple modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return "", fmt.Errorf("decode Apple exponent: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	pub := &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode identity signature: %w", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return "", fmt.Errorf("verify identity signature: %w", err)
	}
	return claims.Subject, nil
}

// handleAppleSignIn exchanges a verified Apple identity token for a
// backend session token. It is deliberately public: it is how a client
// obtains a session, so the auth middleware exempts it (see
// requiresUserAuth). A tombstoned subject (deleted account) cannot
// re-issue: deletion is final until the user creates a new Apple
// identity, which Apple will not do for the same account.
func (s *Server) handleAppleSignIn(w http.ResponseWriter, r *http.Request) {
	secret := authSecret()
	if secret == "" {
		writeErr(w, http.StatusServiceUnavailable, "user authentication is not configured")
		return
	}
	var body struct {
		IdentityToken string `json:"identity_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.IdentityToken) == "" {
		writeErr(w, http.StatusBadRequest, "identity_token is required")
		return
	}
	appleSub, err := verifyAppleIdentityToken(r.Context(), strings.TrimSpace(body.IdentityToken))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid Apple identity token")
		return
	}
	subject := "apple:" + appleSub
	if s.isSubjectDeleted(r.Context(), subject) {
		writeErr(w, http.StatusForbidden, "this account was deleted")
		return
	}
	expires := time.Now().Add(sessionTTL)
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": mintSessionToken(subject, expires, secret),
		"token_type":   "Bearer",
		"expires_in":   int(sessionTTL / time.Second),
		"subject":      subject,
	})
}

// handleDeleteAccount purges the caller's social data and tombstones the
// subject so outstanding bearer tokens stop working and the subject
// cannot sign in again. Published games are NOT deleted: they are public
// content addressed by an opaque identifier that carries no personal
// data, and deleting them would break other users' saves and feeds.
// See docs/AUTH.md for the full deletion semantics.
func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	subject, ok := r.Context().Value(authContextKey{}).(string)
	if !ok || subject == "" {
		writeErr(w, http.StatusUnauthorized, "valid bearer <redacted> is required")
		return
	}
	s.socialMu.Lock()
	delete(s.follows, subject)
	for viewer := range s.follows {
		delete(s.follows[viewer], subject)
	}
	delete(s.blocks, subject)
	for viewer := range s.blocks {
		delete(s.blocks[viewer], subject)
	}
	delete(s.saves, subject)
	delete(s.reactions, subject)
	for gameID, list := range s.comments {
		kept := list[:0]
		for _, c := range list {
			if c.UserID != subject {
				kept = append(kept, c)
			}
		}
		s.comments[gameID] = kept
	}
	delete(s.notInterested, subject)
	delete(s.mutedTopics, subject)
	delete(s.profiles, subject)
	for key, rec := range s.commentIdempotency {
		if rec.UserID == subject {
			delete(s.commentIdempotency, key)
		}
	}
	keptReports := s.reports[:0]
	for _, rep := range s.reports {
		if rep.Reporter != subject {
			keptReports = append(keptReports, rep)
		}
	}
	s.reports = keptReports
	if s.deletedSubjects == nil {
		s.deletedSubjects = map[string]bool{}
	}
	s.deletedSubjects[subject] = true
	persistErr := s.persistSocialStateLocked()
	s.socialMu.Unlock()
	if persistErr != nil {
		writeErr(w, http.StatusServiceUnavailable, "could not persist account deletion")
		return
	}
	if s.socialGraph != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.socialGraph.DeleteUser(ctx, subject); err != nil {
			writeErr(w, http.StatusServiceUnavailable, "could not delete shared social data")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "subject": subject})
}

// isSubjectDeleted reports whether a subject's account was deleted. The
// in-memory tombstone set is authoritative in single-instance mode; when
// a shared Postgres graph is configured it is consulted too, so a
// deletion on another instance still invalidates the token here. A
// Postgres error falls back to the in-memory set rather than locking
// every user out on a transient outage.
func (s *Server) isSubjectDeleted(ctx context.Context, subject string) bool {
	s.socialMu.RLock()
	deleted := s.deletedSubjects[subject]
	s.socialMu.RUnlock()
	if deleted {
		return true
	}
	if s.socialGraph != nil {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if d, err := s.socialGraph.IsUserDeleted(cctx, subject); err == nil && d {
			return true
		}
	}
	return false
}
