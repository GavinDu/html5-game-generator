package api

import (
	"context"
	"os"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestPostgresPreferencesAreSharedAndFilterFeedAcrossInstances(t *testing.T) {
	dsn := os.Getenv("DQ_SOCIAL_DATABASE_URL")
	if dsn == "" {
		t.Skip("DQ_SOCIAL_DATABASE_URL is not configured")
	}
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_AUTH_HMAC_SECRET", "")
	t.Setenv("DQ_SOCIAL_DATABASE_URL", dsn)
	st := store.NewMemoryStore()
	seedReadyGame(st, "pref-game-cats")
	seedReadyGame(st, "pref-game-ai")
	cats, _ := st.GetGame("pref-game-cats")
	cats.Topics = []string{"cats"}
	st.UpdateGame(cats)
	ai, _ := st.GetGame("pref-game-ai")
	ai.Topics = []string{"ai"}
	st.UpdateGame(ai)
	first := New(st, pipeline.NewRunner(st))
	second := New(st, pipeline.NewRunner(st))
	t.Cleanup(func() {
		if first.socialGraph != nil {
			_ = first.socialGraph.ResetPreferences(context.Background(), "pref-viewer")
			_ = first.socialGraph.Close()
		}
		if second.socialGraph != nil {
			_ = second.socialGraph.Close()
		}
	})
	if first.socialGraphErr != nil || second.socialGraphErr != nil {
		t.Fatalf("shared graph initialization failed: first=%v second=%v", first.socialGraphErr, second.socialGraphErr)
	}
	set := do(t, first, "PUT", "/v1/users/me/preferences/topics", map[string]any{"muted_topics": []string{"#Cats"}}, "pref-viewer")
	if set.Code != 200 {
		t.Fatalf("set muted topics: %d %s", set.Code, set.Body.String())
	}
	feedback := do(t, first, "POST", "/v1/games/pref-game-ai/feedback", map[string]any{"kind": "not_interested"}, "pref-viewer")
	if feedback.Code != 200 {
		t.Fatalf("mark not interested: %d %s", feedback.Code, feedback.Body.String())
	}
	var prefs struct {
		MutedTopics []string `json:"muted_topics"`
		NotInterested []string `json:"not_interested_game_ids"`
	}
	got := do(t, second, "GET", "/v1/users/me/preferences", nil, "pref-viewer")
	if got.Code != 200 {
		t.Fatalf("read preferences through second instance: %d %s", got.Code, got.Body.String())
	}
	decode(t, got, &prefs)
	if len(prefs.MutedTopics) != 1 || prefs.MutedTopics[0] != "cats" {
		t.Fatalf("muted topics=%v, want [cats]", prefs.MutedTopics)
	}
	if len(prefs.NotInterested) != 1 || prefs.NotInterested[0] != "pref-game-ai" {
		t.Fatalf("not-interested games=%v, want [pref-game-ai]", prefs.NotInterested)
	}
	feed := do(t, second, "GET", "/v1/feed", nil, "pref-viewer")
	if feed.Code != 200 {
		t.Fatalf("feed with shared preferences: %d %s", feed.Code, feed.Body.String())
	}
	var feedResponse struct { Games []*model.Game `json:"games"` }
	decode(t, feed, &feedResponse)
	for _, game := range feedResponse.Games {
		if game.ID == "pref-game-cats" || game.ID == "pref-game-ai" {
			t.Fatalf("shared preference did not filter %s from feed", game.ID)
		}
	}
	reset := do(t, second, "DELETE", "/v1/users/me/preferences", nil, "pref-viewer")
	if reset.Code != 200 {
		t.Fatalf("reset preferences: %d %s", reset.Code, reset.Body.String())
	}
	got = do(t, first, "GET", "/v1/users/me/preferences", nil, "pref-viewer")
	decode(t, got, &prefs)
	if len(prefs.MutedTopics) != 0 || len(prefs.NotInterested) != 0 {
		t.Fatalf("preferences remain after cross-instance reset: %+v", prefs)
	}
}
