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
	second, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open second graph store: %v", err)
	}

	viewer := "integration-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	creator := "integration-creator-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_ = first.Unfollow(context.Background(), viewer, creator)
		_ = first.Close()
		_ = second.Close()
	})

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
	second, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open second graph store: %v", err)
	}

	viewer := "integration-blocker-" + time.Now().UTC().Format("20060102150405.000000000")
	blocked := "integration-blocked-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_ = first.Unblock(context.Background(), viewer, blocked)
		_ = first.Close()
		_ = second.Close()
	})
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
	second, err := OpenPostgres(ctx, dsn)
	if err != nil {
		t.Fatalf("open second graph store: %v", err)
	}
	viewer := "integration-save-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-saved-game-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_ = first.UnsaveGame(context.Background(), viewer, gameID)
		_ = first.Close()
		_ = second.Close()
	})
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

func TestPostgresStorePersistsModerationReportsAcrossInstances(t *testing.T) {
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
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	report := Report{
		ID: "integration-report-" + suffix, GameID: "integration-game-" + suffix,
		ReporterID: "integration-reporter-" + suffix, Reason: "unsafe asset",
		CreatedAt: time.Now().UTC(), Status: "open",
	}
	t.Cleanup(func() {
		_, _ = first.db.ExecContext(context.Background(), "DELETE FROM social_moderation_reports WHERE id = $1", report.ID)
		_ = first.Close()
		_ = second.Close()
	})
	if err := first.CreateReport(ctx, report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	found, err := second.GetReport(ctx, report.ID)
	if err != nil || !found || found.ID != report.ID || found.Status != "open" {
		t.Fatalf("read report across instances: found=%v report=%+v err=%v", found, found, err)
	}
	items, err := second.ListReports(ctx, "open", report.GameID, 10)
	if err != nil || len(items) != 1 || items[0].ID != report.ID {
		t.Fatalf("list report across instances: reports=%+v err=%v", items, err)
	}
	resolvedAt := time.Now().UTC()
	resolved, changed, err := second.ResolveReport(ctx, report.ID, "moderator-1", "reviewed", resolvedAt)
	if err != nil || !changed || resolved.Status != "resolved" || resolved.ResolvedBy != "moderator-1" {
		t.Fatalf("resolve report: report=%+v changed=%v err=%v", resolved, changed, err)
	}
	repeated, changed, err := first.ResolveReport(ctx, report.ID, "moderator-2", "different reason", time.Now().UTC())
	if err != nil || changed || repeated.ResolvedBy != "moderator-1" || repeated.ResolutionReason != "reviewed" {
		t.Fatalf("repeat resolution changed first decision: report=%+v changed=%v err=%v", repeated, changed, err)
	}
	_, found, err = first.GetReport(ctx, report.ID)
	if err != nil || !found {
		t.Fatalf("report disappeared after resolution: found=%v err=%v", found, err)
	}
}
