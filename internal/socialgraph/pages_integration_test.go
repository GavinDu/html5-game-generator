package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func walkFollowing(t *testing.T, graph *PostgresStore, viewer string, limit int) []string {
	t.Helper()
	ctx := context.Background()
	var all []string
	cursor := ""
	for {
		page, err := graph.ListFollowingPage(ctx, viewer, Page{Limit: limit, Cursor: cursor})
		if err != nil {
			t.Fatalf("list following page cursor=%q: %v", cursor, err)
		}
		all = append(all, page.IDs...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	return all
}

func TestPostgresIDPagesWalkWithoutDuplicatesOrGaps(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	viewer := "integration-page-viewer-" + suffix
	creators := []string{"a-" + suffix, "b-" + suffix, "c-" + suffix, "d-" + suffix, "e-" + suffix}
	for _, creator := range creators {
		if err := graph.Follow(ctx, viewer, creator); err != nil {
			t.Fatalf("follow %s: %v", creator, err)
		}
	}
	t.Cleanup(func() {
		for _, creator := range creators {
			_ = graph.Unfollow(context.Background(), viewer, creator)
		}
	})
	walked := walkFollowing(t, graph, viewer, 2)
	if len(walked) != len(creators) {
		t.Fatalf("walked following = %v, want %v", walked, creators)
	}
	seen := map[string]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Fatalf("duplicate id %s in paged walk", id)
		}
		seen[id] = true
	}
	for _, creator := range creators {
		if !seen[creator] {
			t.Fatalf("paged walk missed creator %s", creator)
		}
	}
	// A cursor from a different viewer must be rejected, not silently reused.
	first, err := graph.ListFollowingPage(ctx, viewer, Page{Limit: 2})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first following page = %+v err=%v, want a next cursor", first, err)
	}
	if _, err := graph.ListFollowingPage(ctx, "other-viewer-"+suffix, Page{Limit: 2, Cursor: first.NextCursor}); err == nil {
		t.Fatalf("cursor reused across viewers: got no error, want fingerprint mismatch")
	}
	// Out-of-range limit is an error.
	if _, err := graph.ListFollowingPage(ctx, viewer, Page{Limit: 51}); err == nil {
		t.Fatalf("following page limit 51: got no error, want an error")
	}
}

func TestPostgresCommentAndReportPagesWalk(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-page-game-" + suffix
	viewer := "integration-page-user-" + suffix
	for i := 0; i < 3; i++ {
		key := "integration-page-key-" + suffix + "-" + string(rune('a'+i))
		if _, _, err := graph.CreateComment(ctx, Comment{ID: "integration-page-comment-" + suffix + "-" + string(rune('a'+i)), GameID: gameID, UserID: viewer, Body: "page test"}, key); err != nil {
			t.Fatalf("create comment %d: %v", i, err)
		}
	}
	var commentIDs []string
	cursor := ""
	for {
		page, err := graph.ListCommentsPage(ctx, gameID, Page{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("list comment page cursor=%q: %v", cursor, err)
		}
		for _, c := range page.Comments {
			commentIDs = append(commentIDs, c.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(commentIDs) != 3 {
		t.Fatalf("walked comments = %v, want 3", commentIDs)
	}
	// Reports, newest first, walked with a filter.
	for i := 0; i < 3; i++ {
		rid := "integration-page-report-" + suffix + "-" + string(rune('a'+i))
		if err := graph.CreateReport(ctx, Report{ID: rid, GameID: gameID, ReporterID: viewer, Reason: "page test", CreatedAt: time.Now().UTC(), Status: "open"}); err != nil {
			t.Fatalf("create report %d: %v", i, err)
		}
	}
	var reportIDs []string
	cursor = ""
	for {
		page, err := graph.ListReportsPage(ctx, "open", gameID, Page{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatalf("list report page cursor=%q: %v", cursor, err)
		}
		for _, r := range page.Reports {
			reportIDs = append(reportIDs, r.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(reportIDs) != 3 {
		t.Fatalf("walked reports = %v, want 3", reportIDs)
	}
}
