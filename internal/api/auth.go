package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type authClaims struct {
	Subject string `json:"sub"`
	Expires int64  `json:"exp"`
	Issuer  string `json:"iss"`
}

type authContextKey struct{}

func verifyBearerUserID(r *http.Request) string {
	secret := strings.TrimSpace(os.Getenv("DQ_AUTH_HMAC_SECRET"))
	if secret == "" {
		return ""
	}
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(raw, "Bearer ") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(raw, "Bearer "), ".")
	if len(parts) != 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return ""
	}
	var claims authClaims
	if json.Unmarshal(payload, &claims) != nil ||
		strings.TrimSpace(claims.Subject) == "" ||
		len(claims.Subject) > 128 ||
		claims.Issuer != "hephaestus" ||
		claims.Expires <= time.Now().Unix() {
		return ""
	}
	return claims.Subject
}

// requireAuthenticatedUser is enabled when a signing secret is configured.
// Public discovery and content reads remain anonymous; user-specific API
// operations require a verified, expiring bearer token. X-User-ID is never
// accepted as an identity in this mode.
func requireAuthenticatedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresUserAuth(r) {
			// Public reads may be anonymous, but a valid optional bearer token
			// still supplies the verified owner identity for private-resource checks.
			if subject := verifyBearerUserID(r); subject != "" {
				ctx := context.WithValue(r.Context(), authContextKey{}, subject)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.TrimSpace(os.Getenv("DQ_AUTH_HMAC_SECRET")) == "" {
			if strings.EqualFold(strings.TrimSpace(os.Getenv("DQ_ALLOW_INSECURE_USER_ID")), "true") {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusServiceUnavailable, "user authentication is not configured")
			return
		}
		sub := verifyBearerUserID(r)
		if sub == "" {
			writeErr(w, http.StatusUnauthorized, "valid bearer authentication is required")
			return
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, sub)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requiresUserAuth(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/v1/telemetry/") ||
		r.URL.Path == "/v1/moderation/contact" ||
		r.URL.Path == "/v1/moderation/reports" ||
		r.URL.Path == "/v1/moderation/audit" ||
		strings.HasSuffix(r.URL.Path, "/moderation") ||
		strings.Contains(r.URL.Path, "/moderation/reports/") ||
		strings.HasSuffix(r.URL.Path, "/quarantine") ||
		strings.HasSuffix(r.URL.Path, "/unquarantine") {
		return false // these endpoints have their own trust model or are ingestion-only
	}
	if r.Method == http.MethodGet {
		p := r.URL.Path
		if p == "/v1/feed" || p == "/v1/creators" ||
			strings.HasPrefix(p, "/v1/games/") ||
			strings.HasPrefix(p, "/v1/users/") && p != "/v1/users/me/following" && p != "/v1/users/me/saved" && p != "/v1/users/me/preferences" ||
			strings.HasPrefix(p, "/v1/groups/") && strings.HasSuffix(p, "/games") ||
			strings.HasPrefix(p, "/v1/runs/") ||
			strings.HasPrefix(p, "/v1/jobs/") ||
			strings.HasPrefix(p, "/v1/ir/") || strings.HasPrefix(p, "/v1/game-ir/") {
			return false
		}
	}
	return true
}

// authTokenForTest signs a short-lived bearer token with the same contract as
// the verifier. It is deliberately not an HTTP endpoint; production token
// issuance must be performed by a trusted identity provider.
func authTokenForTest(subject string, expires time.Time, secret string) string {
	payload, _ := json.Marshal(authClaims{Subject: subject, Expires: expires.Unix(), Issuer: "hephaestus"})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
