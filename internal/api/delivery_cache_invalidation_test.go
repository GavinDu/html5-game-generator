package api

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

type recordingDeliveryInvalidator struct {
	mu  sync.Mutex
	ids []string
	err error
}

func (f *recordingDeliveryInvalidator) InvalidateGame(_ context.Context, gameID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, gameID)
	return f.err
}

func (f *recordingDeliveryInvalidator) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ids...)
}

func TestDeliveryCacheInvalidatesAfterVisibilityMutation(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	fake := &recordingDeliveryInvalidator{}
	srv.SetDeliveryCacheInvalidator(fake)
	game := seedReadyGame(st, "invalidate-visibility")

	rec := do(t, srv, http.MethodPost, "/v1/games/"+game.ID+"/visibility", map[string]any{
		"visibility": "private",
	}, game.OwnerID)
	if rec.Code != http.StatusOK {
		t.Fatalf("visibility mutation: got %d: %s", rec.Code, rec.Body.String())
	}
	if got := fake.calls(); len(got) != 1 || got[0] != game.ID {
		t.Fatalf("invalidations = %v, want [%s]", got, game.ID)
	}
}

func TestDeliveryCacheInvalidatesAfterModerationMutation(t *testing.T) {
	t.Setenv("DQ_ALLOW_INSECURE_USER_ID", "true")
	t.Setenv("DQ_MODERATION_TOKEN", "moderation-test-token")
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	fake := &recordingDeliveryInvalidator{}
	srv.SetDeliveryCacheInvalidator(fake)
	game := seedReadyGame(st, "invalidate-moderation")

	rec := do(t, srv, http.MethodPost, "/v1/games/"+game.ID+"/moderation", map[string]any{
		"status": "rejected",
		"reason": "policy review",
	}, "moderator")
	if rec.Code != http.StatusOK {
		t.Fatalf("moderation mutation: got %d: %s", rec.Code, rec.Body.String())
	}
	if got := fake.calls(); len(got) != 1 || got[0] != game.ID {
		t.Fatalf("invalidations = %v, want [%s]", got, game.ID)
	}

	// Invalid requests are rejected before the game is changed and must not
	// cause a purge that could mask an authorization or validation bug.
	rec = do(t, srv, http.MethodPost, "/v1/games/"+game.ID+"/moderation", map[string]any{
		"status": "not-a-status",
	}, "moderator")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid moderation mutation: got %d", rec.Code)
	}
	if got := fake.calls(); len(got) != 1 {
		t.Fatalf("invalid request changed invalidation count: %v", got)
	}
}
