package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestCreatorProfileUpdateValidationAndPersistence(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(t.TempDir(), "social-state.json"))
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))

	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("X-User-ID", "viewer-1")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		srv.Routes().ServeHTTP(rec, req)
		return rec
	}

	invalid := request(http.MethodPut, "/v1/users/me/profile", `{"display_name":"Creator","avatar_url":"http://example.test/avatar.png"}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("HTTP avatar URL status = %d, want 400: %s", invalid.Code, invalid.Body.String())
	}
	tooLong := request(http.MethodPut, "/v1/users/me/profile", `{"display_name":"123456789012345678901234567890123456789012345678901"}`)
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("long display name status = %d, want 400: %s", tooLong.Code, tooLong.Body.String())
	}

	updated := request(http.MethodPut, "/v1/users/me/profile", `{"display_name":"  Orbit Creator  ","avatar_url":"https://cdn.example.test/avatar.png"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("profile update status = %d: %s", updated.Code, updated.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(updated.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["display_name"] != "Orbit Creator" || response["avatar_url"] != "https://cdn.example.test/avatar.png" {
		t.Fatalf("profile response = %#v", response)
	}

	restarted := New(st, pipeline.NewRunner(st))
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me/profile", nil)
	req.Header.Set("X-User-ID", "viewer-1")
	rec := httptest.NewRecorder()
	restarted.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile read after restart status = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["display_name"] != "Orbit Creator" || response["avatar_url"] != "https://cdn.example.test/avatar.png" {
		t.Fatalf("profile did not persist: %#v", response)
	}
}
