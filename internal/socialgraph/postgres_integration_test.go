package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreSharesFollowEdgesAcrossInstances(t *testing.T) {
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
	defer first.Close()
	second, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open second graph store: %v", err)
	}
	defer second.Close()

	viewer := "integration-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	creator := "integration-creator-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() { _ = first.Unfollow(context.Background(), viewer, creator) })

	if err := first.Follow(ctx, viewer, creator); err != nil {
		t.Fatalf("follow through first instance: %v", err)
	}
	ids, err := second.ListFollowing(ctx, viewer)
	if err != nil {
		t.Fatalf("read through second instance: %v", err)
	}
	if len(ids) != 1 || ids[0] != creator {
		t.Fatalf("second instance saw following=%v, want [%s]", ids, creator)
	}
	if err := second.Unfollow(ctx, viewer, creator); err != nil {
		t.Fatalf("unfollow through second instance: %v", err)
	}
	ids, err = first.ListFollowing(ctx, viewer)
	if err != nil {
		t.Fatalf("read after cross-instance unfollow: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("first instance still sees following=%v after unfollow", ids)
	}
}
