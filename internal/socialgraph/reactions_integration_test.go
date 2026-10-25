package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreSharesReactionStateAcrossInstances(t *testing.T) {
	dsn := os.Getenv("DQ_SOCIAL_DATABASE_URL")
	if dsn == "" {
		t.Skip("DQ_SOCIAL_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open first graph store: %v", err)
	}
	second, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open second graph store: %v", err)
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	viewer := "integration-reaction-viewer-" + suffix
	other := "integration-reaction-other-" + suffix
	gameID := "integration-reaction-game-" + suffix
	t.Cleanup(func() {
		_ = first.DeleteReaction(context.Background(), viewer, gameID)
		_ = first.DeleteReaction(context.Background(), other, gameID)
		_ = first.Close()
		_ = second.Close()
	})

	if err := first.SetReaction(ctx, viewer, gameID, "like"); err != nil {
		t.Fatalf("set initial reaction: %v", err)
	}
	if err := first.SetReaction(ctx, viewer, gameID, "love"); err != nil {
		t.Fatalf("update reaction idempotently: %v", err)
	}
	if err := second.SetReaction(ctx, other, gameID, "funny"); err != nil {
		t.Fatalf("set second viewer reaction: %v", err)
	}

	counts, reactions, err := second.ListReactionSummary(ctx, viewer, []string{gameID, gameID})
	if err != nil {
		t.Fatalf("read reaction summary from second instance: %v", err)
	}
	if counts[gameID] != 2 || reactions[gameID] != "love" {
		t.Fatalf("reaction summary counts=%v viewer=%v, want count 2 and love", counts, reactions)
	}

	if err := second.DeleteReaction(ctx, other, gameID); err != nil {
		t.Fatalf("delete reaction through second instance: %v", err)
	}
	counts, reactions, err = first.ListReactionSummary(ctx, viewer, []string{gameID})
	if err != nil {
		t.Fatalf("read after delete through first instance: %v", err)
	}
	if counts[gameID] != 1 || reactions[gameID] != "love" {
		t.Fatalf("reaction summary after delete counts=%v viewer=%v, want count 1 and love", counts, reactions)
	}
	if err := first.SetReaction(ctx, viewer, gameID, "invalid"); err == nil {
		t.Fatal("invalid reaction type must be rejected")
	}
}
