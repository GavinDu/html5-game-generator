package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

func TestCommentIdempotencyAndTopicPreferenceControls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	srv := newTestServer()
	addPublicGame(srv, "topic-puzzle", "creator")
	addPublicGame(srv, "topic-racing", "racer")
	puzzle, _ := srv.store.GetGame("topic-puzzle")
	puzzle.Topics = []string{"Puzzle"}
	srv.store.UpdateGame(puzzle)
	racing, _ := srv.store.GetGame("topic-racing")
	racing.Topics = []string{"racing"}
	srv.store.UpdateGame(racing)

	first := doWithHeaders(t, srv, http.MethodPost, "/v1/games/topic-puzzle/comments", map[string]any{"body":"Great!"}, "viewer", map[string]string{"Idempotency-Key":"comment-1"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first comment: %d %s", first.Code, first.Body.String())
	}
	second := doWithHeaders(t, srv, http.MethodPost, "/v1/games/topic-puzzle/comments", map[string]any{"body":"Great!"}, "viewer", map[string]string{"Idempotency-Key":"comment-1"})
	if second.Code != http.StatusCreated || !strings.Contains(second.Body.String(), `"idempotent_replay":true`) {
		t.Fatalf("replayed comment: %d %s", second.Code, second.Body.String())
	}
	conflict := doWithHeaders(t, srv, http.MethodPost, "/v1/games/topic-puzzle/comments", map[string]any{"body":"Different"}, "viewer", map[string]string{"Idempotency-Key":"comment-1"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict: %d %s", conflict.Code, conflict.Body.String())
	}
	comments := do(t, srv, http.MethodGet, "/v1/games/topic-puzzle/comments", nil, "viewer")
	if strings.Count(comments.Body.String(), `"id":"`) != 1 {
		t.Fatalf("duplicate comment persisted: %s", comments.Body.String())
	}

	prefs := do(t, srv, http.MethodPut, "/v1/users/me/preferences/topics", map[string]any{"muted_topics":[]string{"#PUZZLE"}}, "viewer")
	if prefs.Code != http.StatusOK {
		t.Fatalf("set topic preference: %d %s", prefs.Code, prefs.Body.String())
	}
	feed := do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
	if strings.Contains(feed.Body.String(), "topic-puzzle") || !strings.Contains(feed.Body.String(), "topic-racing") {
		t.Fatalf("topic preference did not filter feed: %s", feed.Body.String())
	}

	reset := do(t, srv, http.MethodDelete, "/v1/users/me/preferences", nil, "viewer")
	if reset.Code != http.StatusOK {
		t.Fatalf("reset preferences: %d %s", reset.Code, reset.Body.String())
	}
	feed = do(t, srv, http.MethodGet, "/v1/feed?scope=for_you", nil, "viewer")
	if !strings.Contains(feed.Body.String(), "topic-puzzle") {
		t.Fatalf("reset did not restore feed item: %s", feed.Body.String())
	}
}

func TestCommentIdempotencySurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	first := newTestServer()
	addPublicGame(first, "persist-idempotency", "creator")
	resp := doWithHeaders(t, first, http.MethodPost, "/v1/games/persist-idempotency/comments", map[string]any{"body":"Once"}, "viewer", map[string]string{"Idempotency-Key":"persist-key"})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.Code, resp.Body.String())
	}
	second := newTestServer()
	addPublicGame(second, "persist-idempotency", "creator")
	resp = doWithHeaders(t, second, http.MethodPost, "/v1/games/persist-idempotency/comments", map[string]any{"body":"Once"}, "viewer", map[string]string{"Idempotency-Key":"persist-key"})
	if resp.Code != http.StatusCreated || !strings.Contains(resp.Body.String(), `"idempotent_replay":true`) {
		t.Fatalf("restart replay: %d %s", resp.Code, resp.Body.String())
	}
}

func doWithHeaders(t *testing.T, srv *Server, method, path string, body any, user string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if user != "" {
		req.Header.Set("X-User-ID", user)
	}
	if token := os.Getenv("DQ_MODERATION_TOKEN"); token != "" {
		req.Header.Set("X-Moderation-Token", token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}
