package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Backend route-contract evidence for Hephaestus successor Epic #642/#664.
// The real social handler patterns are registered in social.go. This test
// asserts Go 1.24 ServeMux's *path-segment* decoding for the exact native
// namespaced ID encodings; it does not authenticate a creator or make a
// production account public.
func TestCreatorPathSegmentNamespacedRouteContract(t *testing.T) {
	mux := http.NewServeMux()
	// Match the shipping registerSocialRoutes method/pattern grammar.
	for _, route := range []string{
		"GET /v1/users/{id}",
		"POST /v1/users/{id}/follow",
		"DELETE /v1/users/{id}/follow",
		"POST /v1/users/{id}/block",
		"DELETE /v1/users/{id}/block",
	} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test-Route-ID", r.PathValue("id"))
			w.WriteHeader(http.StatusNoContent)
		})
	}
	type endpoint struct{ method, suffix string }
	for _, endpoint := range []endpoint{
		{"GET", ""}, {"POST", "/follow"}, {"DELETE", "/follow"},
		{"POST", "/block"}, {"DELETE", "/block"},
	} {
		for _, tc := range []struct {
			encoded, identity string
		}{
			{"creator-123_ABC", "creator-123_ABC"},
			{"apple%3AprivateSubject", "apple:privateSubject"},
			{"apple%3A%C3%A9", "apple:é"},
			{"apple%3A%E5%B0%8F%E6%98%8E", "apple:小明"},
			{"user%2Bother", "user+other"},
			{"user%26admin", "user&admin"},
			{"auth0%7Csubject", "auth0|subject"},
			{"google-oauth2%7C123", "google-oauth2|123"},
			{"github%3A123%40id", "github:123@id"},
			{"user.name~2", "user.name~2"},
			{strings.Repeat("a", 128), strings.Repeat("a", 128)},
		} {
			target := "https://api.example.test/v1/users/" + tc.encoded + endpoint.suffix
			req := httptest.NewRequest(endpoint.method, target, nil)
			resp := httptest.NewRecorder()
			mux.ServeHTTP(resp, req)
			if resp.Code != http.StatusNoContent {
				t.Fatalf("%s %s: expected route to match, got %d", endpoint.method, target, resp.Code)
			}
			if got := resp.Header().Get("X-Test-Route-ID"); got != tc.identity {
				t.Fatalf("%s %s: decoded id=%q, want %q", endpoint.method, target, got, tc.identity)
			}
			if req.URL.RawQuery != "" || req.URL.Fragment != "" {
				t.Fatalf("%s %s: canonical creator route acquired query or fragment", endpoint.method, target)
			}
		}
	}
}

func TestCreatorPathSegmentsDoNotRouteRawSlashOrInjectedSuffix(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	for _, path := range []string{
		"/v1/users/creator/other",
		"/v1/users/../users/other",
		"/v1/users/creator/follow",
	} {
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest("GET", "https://api.example.test"+path, nil))
		if resp.Code == http.StatusNoContent {
			t.Fatalf("raw multi-segment creator path unexpectedly matched one id: %q", path)
		}
	}
}

// Unlike the route-grammar matrix above, this invokes the *real shipping*
// social handler registration and handler. A public, empty creator-profile
// response demonstrates identity-preserving routing only; it does NOT
// fabricate a creator, published game, authorization, or social relationship.
func TestProductionCreatorProfileRoutePreservesEscapedOpaqueID(t *testing.T) {
	st := store.NewMemoryStore()
	server := New(st, pipeline.NewRunner(st))
	mux := http.NewServeMux()
	server.registerSocialRoutes(mux)

	for _, tc := range []struct {
		encoded, identity string
	}{
		{"apple%3AprivateSubject", "apple:privateSubject"},
		{"apple%3A%C3%A9", "apple:é"},
		{"apple%3A%E5%B0%8F%E6%98%8E", "apple:小明"},
		{"auth0%7Csubject", "auth0|subject"},
		{"github%3A123%40id", "github:123@id"},
		{"creator-123_ABC", "creator-123_ABC"},
	} {
		req := httptest.NewRequest("GET", "https://api.example.test/v1/users/"+tc.encoded, nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("real creator profile route rejected encoded path: HTTP %d", resp.Code)
		}
		var body struct {
			CreatorID string `json:"creator_id"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
			t.Fatalf("real creator profile response shape invalid: %v", err)
		}
		if body.CreatorID != tc.identity {
			t.Fatalf("real handler changed encoded identity: got %q want %q", body.CreatorID, tc.identity)
		}
	}
}
