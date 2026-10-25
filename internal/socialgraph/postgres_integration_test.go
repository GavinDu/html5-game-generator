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

func TestPostgresStoreSharesBlockRelationsAcrossInstances(t *testing.T) {
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

	viewer := "integration-blocker-" + time.Now().UTC().Format("20060102150405.000000000")
	blocked := "integration-blocked-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() { _ = first.Unblock(context.Background(), viewer, blocked) })
	inserted, err := first.ImportBlocks(ctx, []BlockRelation{{ViewerID: viewer, BlockedID: blocked}})
	if err != nil || inserted != 1 {
		t.Fatalf("initial block import: inserted=%d err=%v, want 1 row", inserted, err)
	}
	inserted, err = second.ImportBlocks(ctx, []BlockRelation{{ViewerID: viewer, BlockedID: blocked}})
	if err != nil || inserted != 0 {
		t.Fatalf("resumed block import: inserted=%d err=%v, want 0 new rows", inserted, err)
	}
	if err := first.Block(ctx, viewer, blocked); err != nil {
		t.Fatalf("block through first instance: %v", err)
	}
	if err := first.Block(ctx, viewer, blocked); err != nil {
		t.Fatalf("idempotent repeated block: %v", err)
	}
	ids, err := second.ListBlocked(ctx, viewer)
	if err != nil {
		t.Fatalf("read blocks through second instance: %v", err)
	}
	if len(ids) != 1 || ids[0] != blocked {
		t.Fatalf("second instance saw blocked=%v, want [%s]", ids, blocked)
	}
	if err := second.Unblock(ctx, viewer, blocked); err != nil {
		t.Fatalf("unblock through second instance: %v", err)
	}
	ids, err = first.ListBlocked(ctx, viewer)
	if err != nil {
		t.Fatalf("read after cross-instance unblock: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("first instance still sees blocks=%v after unblock", ids)
	}
}

func TestPostgresStoreSharesSavedGamesAcrossInstances(t *testing.T) {
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
	viewer := "integration-save-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-saved-game-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() { _ = first.UnsaveGame(context.Background(), viewer, gameID) })
	inserted, err := first.ImportSaves(ctx, []SaveRelation{{ViewerID: viewer, GameID: gameID}})
	if err != nil || inserted != 1 {
		t.Fatalf("initial save import: inserted=%d err=%v, want 1 row", inserted, err)
	}
	inserted, err = second.ImportSaves(ctx, []SaveRelation{{ViewerID: viewer, GameID: gameID}})
	if err != nil || inserted != 0 {
		t.Fatalf("resumed save import: inserted=%d err=%v, want 0 new rows", inserted, err)
	}
	if err := first.SaveGame(ctx, viewer, gameID); err != nil {
		t.Fatalf("idempotent save: %v", err)
	}
	ids, err := second.ListSavedGames(ctx, viewer)
	if err != nil {
		t.Fatalf("read saved games through second instance: %v", err)
	}
	if len(ids) != 1 || ids[0] != gameID {
		t.Fatalf("second instance saw saved games=%v, want [%s]", ids, gameID)
	}
	if err := second.UnsaveGame(ctx, viewer, gameID); err != nil {
		t.Fatalf("un-save through second instance: %v", err)
	}
	ids, err = first.ListSavedGames(ctx, viewer)
	if err != nil {
		t.Fatalf("read after cross-instance un-save: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("first instance still sees saved games=%v", ids)
	}
}
