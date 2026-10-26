package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresListAllIDsReturnsFullSets(t *testing.T) {
	dsn := os.Getenv("DQ_SOCIAL_DATABASE_URL")
	if dsn == "" {
		t.Skip("DQ_SOCIAL_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	graph, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open graph store: %v", err)
	}
	defer graph.Close()

	viewer, creator := "allids-viewer", "allids-creator"
	if err := graph.Follow(ctx, viewer, creator); err != nil {
		t.Fatalf("follow: %v", err)
	}
	if err := graph.Block(ctx, viewer, "allids-blocked"); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := graph.SaveGame(ctx, viewer, "allids-game"); err != nil {
		t.Fatalf("save game: %v", err)
	}
	if err := graph.MarkNotInterested(ctx, viewer, "allids-ignored"); err != nil {
		t.Fatalf("mark not interested: %v", err)
	}
	if err := graph.SetMutedTopics(ctx, viewer, []string{"allids-topic"}); err != nil {
		t.Fatalf("set muted topics: %v", err)
	}

	all, err := graph.ListAllIDs(ctx, viewer)
	if err != nil {
		t.Fatalf("list all ids: %v", err)
	}
	contains := func(set []string, want string) bool {
		for _, id := range set {
			if id == want {
				return true
			}
		}
		return false
	}
	if !contains(all.Following, creator) {
		t.Errorf("following %v does not contain %q", all.Following, creator)
	}
	if !contains(all.Blocked, "allids-blocked") {
		t.Errorf("blocked %v does not contain %q", all.Blocked, "allids-blocked")
	}
	if !contains(all.SavedGames, "allids-game") {
		t.Errorf("saved %v does not contain %q", all.SavedGames, "allids-game")
	}
	if !contains(all.NotInterested, "allids-ignored") {
		t.Errorf("not interested %v does not contain %q", all.NotInterested, "allids-ignored")
	}
	if !contains(all.MutedTopics, "allids-topic") {
		t.Errorf("muted topics %v does not contain %q", all.MutedTopics, "allids-topic")
	}
}
