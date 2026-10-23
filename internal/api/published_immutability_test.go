package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestPublishedPostsCannotBeRepairedOrTuned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DQ_SOCIAL_STATE_FILE", filepath.Join(dir, "social-state.json"))
	t.Setenv("DQ_SOCIAL_EVENTS_FILE", filepath.Join(dir, "social-events.jsonl"))
	t.Setenv("DQ_TELEMETRY_DIR", filepath.Join(dir, "telemetry"))
	t.Setenv("DQ_MODERATION_TOKEN", "test-moderator-token")

	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "published-immutable")
	publishedAt := time.Now().UTC()
	game.PublishedAt = &publishedAt
	game.PublishedVersion = 1
	game.Caption = "Published caption"
	game.Topics = []string{"puzzle"}
	st.UpdateGame(game)

	// Seed real pending proposals so apply routes reach the published-state
	// guard rather than passing because a proposal is missing.
	srv.repairProposals = map[string]*repairProposal{
		"published-immutable": {
			GameID: "published-immutable",
			Status: "proposed",
		},
	}
	srv.tunerProposals = map[string]*tunerProposal{
		"test": {
			ID: "test", GameID: "published-immutable",
			Knob: "health", From: 1, To: 2, Status: "proposed",
		},
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "repair proposal", path: "/v1/games/published-immutable/repair/propose"},
		{name: "repair apply", path: "/v1/games/published-immutable/repair/apply"},
		{name: "tuning proposal", path: "/v1/games/published-immutable/tuner/propose"},
		{name: "tuning apply", path: "/v1/games/published-immutable/tuner/proposals/test/apply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set("X-Moderation-Token", "test-moderator-token")
			rec := httptest.NewRecorder()
			srv.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusConflict {
				t.Fatalf("published post mutation should be rejected: got %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
			}
		})
	}

	after, ok := st.GetGame("published-immutable")
	if !ok {
		t.Fatal("published game disappeared")
	}
	if after.PublishedAt == nil || !after.PublishedAt.Equal(publishedAt) || after.PublishedVersion != 1 {
		t.Fatalf("publication metadata changed: published_at=%v version=%d", after.PublishedAt, after.PublishedVersion)
	}
	if after.Caption != "Published caption" || len(after.Topics) != 1 || after.Topics[0] != "puzzle" {
		t.Fatalf("published social metadata changed: caption=%q topics=%v", after.Caption, after.Topics)
	}
}
