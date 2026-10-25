package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresPreferencesSharedAcrossInstances(t *testing.T) {
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
		_ = first.Close()
		t.Fatalf("open second graph store: %v", err)
	}
	viewer := "integration-preferences-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-not-interested-game-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_ = first.ResetPreferences(context.Background(), viewer)
		_ = first.Close()
		_ = second.Close()
	})
	if err := first.SetMutedTopics(ctx, viewer, []string{"#Cats", "AI", "cats"}); err != nil {
		t.Fatalf("set muted topics: %v", err)
	}
	if err := first.AddNotInterested(ctx, viewer, gameID); err != nil {
		t.Fatalf("add not-interested game: %v", err)
	}
	prefs, err := second.ListPreferences(ctx, viewer)
	if err != nil {
		t.Fatalf("read preferences through second instance: %v", err)
	}
	if len(prefs.MutedTopics) != 2 || prefs.MutedTopics[0] != "ai" || prefs.MutedTopics[1] != "cats" {
		t.Fatalf("muted topics=%v, want [ai cats]", prefs.MutedTopics)
	}
	if len(prefs.NotInterestedGameIDs) != 1 || prefs.NotInterestedGameIDs[0] != gameID {
		t.Fatalf("not-interested games=%v, want [%s]", prefs.NotInterestedGameIDs, gameID)
	}
	if err := second.ResetPreferences(ctx, viewer); err != nil {
		t.Fatalf("reset preferences: %v", err)
	}
	prefs, err = first.ListPreferences(ctx, viewer)
	if err != nil {
		t.Fatalf("read after reset: %v", err)
	}
	if len(prefs.MutedTopics) != 0 || len(prefs.NotInterestedGameIDs) != 0 {
		t.Fatalf("preferences remained after reset: %+v", prefs)
	}
}
