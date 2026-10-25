package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreSharesFeedPreferencesAcrossInstances(t *testing.T) {
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
	viewer := "integration-pref-viewer-" + suffix
	gameID := "integration-pref-game-" + suffix
	t.Cleanup(func() {
		_ = first.ResetPreferences(context.Background(), viewer)
		_ = first.Close()
		_ = second.Close()
	})

	if err := first.MarkNotInterested(ctx, viewer, gameID); err != nil {
		t.Fatalf("mark not interested through first instance: %v", err)
	}
	if err := first.MarkNotInterested(ctx, viewer, gameID); err != nil {
		t.Fatalf("repeat not-interested mutation: %v", err)
	}
	if err := first.SetMutedTopics(ctx, viewer, []string{"#Space", " Cozy ", "space"}); err != nil {
		t.Fatalf("set muted topics: %v", err)
	}

	ignored, err := second.ListNotInterested(ctx, viewer)
	if err != nil || len(ignored) != 1 || ignored[0] != gameID {
		t.Fatalf("second instance saw not-interested=%v err=%v, want [%s]", ignored, err, gameID)
	}
	muted, err := second.ListMutedTopics(ctx, viewer)
	if err != nil || len(muted) != 2 || muted[0] != "cozy" || muted[1] != "space" {
		t.Fatalf("second instance saw muted topics=%v err=%v, want [cozy space]", muted, err)
	}

	if err := second.SetMutedTopics(ctx, viewer, []string{"art"}); err != nil {
		t.Fatalf("replace muted topics through second instance: %v", err)
	}
	muted, err = first.ListMutedTopics(ctx, viewer)
	if err != nil || len(muted) != 1 || muted[0] != "art" {
		t.Fatalf("first instance saw muted topics after replacement=%v err=%v, want [art]", muted, err)
	}

	if err := second.ResetPreferences(ctx, viewer); err != nil {
		t.Fatalf("reset preferences through second instance: %v", err)
	}
	ignored, err = first.ListNotInterested(ctx, viewer)
	if err != nil || len(ignored) != 0 {
		t.Fatalf("first instance saw not-interested after reset=%v err=%v", ignored, err)
	}
	muted, err = first.ListMutedTopics(ctx, viewer)
	if err != nil || len(muted) != 0 {
		t.Fatalf("first instance saw muted topics after reset=%v err=%v", muted, err)
	}
}
