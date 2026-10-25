package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestMutualBlocksHideGameFromFeedAndCreatorDiscovery(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	st := store.NewMemoryStore()
	game := feedFixtureGame("blocked-game", "blocked-creator", time.Now().UTC())
	st.CreateGame(game)
	srv := New(st, pipeline.NewRunner(st))
	srv.socialMu.Lock()
	srv.blocks["blocked-creator"] = map[string]bool{"viewer": true}
	srv.socialMu.Unlock()

	feedReq := httptest.NewRequest(http.MethodGet, "/v1/feed?scope=for_you", nil)
	feedReq.Header.Set("X-User-ID", "viewer")
	feedRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(feedRec, feedReq)
	if feedRec.Code != http.StatusOK {
		t.Fatalf("feed returned %d: %s", feedRec.Code, feedRec.Body.String())
	}
	var feed struct {
		Games []map[string]any `json:"games"`
	}
	if err := json.Unmarshal(feedRec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Games) != 0 {
		t.Fatalf("feed must hide creators who blocked viewer: %#v", feed.Games)
	}

	discoverReq := httptest.NewRequest(http.MethodGet, "/v1/creators", nil)
	discoverReq.Header.Set("X-User-ID", "viewer")
	discoverRec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(discoverRec, discoverReq)
	if discoverRec.Code != http.StatusOK {
		t.Fatalf("creator discovery returned %d: %s", discoverRec.Code, discoverRec.Body.String())
	}
	var discovery struct {
		Creators []map[string]any `json:"creators"`
	}
	if err := json.Unmarshal(discoverRec.Body.Bytes(), &discovery); err != nil {
		t.Fatal(err)
	}
	if len(discovery.Creators) != 0 {
		t.Fatalf("discovery must hide creators who blocked viewer: %#v", discovery.Creators)
	}
	if srv.canDeliver(game, "viewer") {
		t.Fatal("package delivery must deny a creator who blocked the viewer")
	}
	if !srv.canDeliver(game, "blocked-creator") {
		t.Fatal("creator must retain access to their own game")
	}
}
