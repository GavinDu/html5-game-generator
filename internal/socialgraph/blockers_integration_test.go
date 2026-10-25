package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresStoreListsBlockersAcrossInstances(t *testing.T) {
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
	blocker := "integration-blocker-" + suffix
	viewer := "integration-viewer-blocked-" + suffix
	t.Cleanup(func() {
		_ = first.Unblock(context.Background(), blocker, viewer)
		_ = first.Close()
		_ = second.Close()
	})
	if err := first.Block(ctx, blocker, viewer); err != nil {
		t.Fatalf("block through first instance: %v", err)
	}
	ids, err := second.ListBlockers(ctx, viewer)
	if err != nil {
		t.Fatalf("read reverse block relation through second instance: %v", err)
	}
	if len(ids) != 1 || ids[0] != blocker {
		t.Fatalf("second instance saw blockers=%v, want [%s]", ids, blocker)
	}
}
