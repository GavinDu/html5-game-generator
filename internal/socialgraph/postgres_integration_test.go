package socialgraph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

func TestPostgresCommentsAreSharedAndIdempotentAcrossInstances(t *testing.T) {
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
	viewer := "integration-comment-user-" + suffix
	gameID := "integration-comment-game-" + suffix
	key := "integration-comment-key-" + suffix
	commentID := "integration-comment-" + suffix
	t.Cleanup(func() {
		_, _, _ = first.DeleteComment(context.Background(), gameID, commentID, viewer)
		_ = first.Close()
		_ = second.Close()
	})

	created, replay, err := first.CreateComment(ctx, Comment{
		ID: commentID, GameID: gameID, UserID: viewer, Body: "hello shared graph",
	}, key)
	if err != nil || replay {
		t.Fatalf("create comment: replay=%v err=%v", replay, err)
	}
	comments, err := second.ListComments(ctx, gameID)
	if err != nil {
		t.Fatalf("list comments through second instance: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != created.ID || comments[0].Body != "hello shared graph" {
		t.Fatalf("second instance saw comments=%+v, want created comment", comments)
	}
	backupPath := filepath.Join(t.TempDir(), "social-graph.json")
	if err := first.BackupTo(ctx, backupPath); err != nil {
		t.Fatalf("backup social graph with comments: %v", err)
	}

	replayed, wasReplay, err := second.CreateComment(ctx, Comment{
		ID: "duplicate-" + suffix, GameID: gameID, UserID: viewer, Body: "hello shared graph",
	}, key)
	if err != nil || !wasReplay || replayed.ID != created.ID {
		t.Fatalf("idempotent replay = %+v replay=%v err=%v", replayed, wasReplay, err)
	}
	_, _, err = second.CreateComment(ctx, Comment{
		ID: "conflict-" + suffix, GameID: gameID, UserID: viewer, Body: "different body",
	}, key)
	if !errors.Is(err, ErrCommentIdempotencyConflict) {
		t.Fatalf("conflicting idempotency key error = %v, want ErrCommentIdempotencyConflict", err)
	}

	_, _, err = second.CreateComment(ctx, Comment{
		ID: "other-user-" + suffix, GameID: gameID, UserID: "other-user-" + suffix, Body: "hello shared graph",
	}, key)
	if !errors.Is(err, ErrCommentIdempotencyConflict) {
		t.Fatalf("cross-user idempotency key error = %v, want ErrCommentIdempotencyConflict", err)
	}

	found, owned, err := second.DeleteComment(ctx, gameID, created.ID, "different-user")
	if err != nil || !found || owned {
		t.Fatalf("non-author delete = found %v owned %v err %v", found, owned, err)
	}
	found, owned, err = second.DeleteComment(ctx, gameID, created.ID, viewer)
	if err != nil || !found || !owned {
		t.Fatalf("author delete = found %v owned %v err %v", found, owned, err)
	}
	comments, err = first.ListComments(ctx, gameID)
	if err != nil || len(comments) != 0 {
		t.Fatalf("first instance still sees comments after delete: comments=%+v err=%v", comments, err)
	}
	if err := first.RestoreFrom(ctx, backupPath); err != nil {
		t.Fatalf("restore social graph with comments: %v", err)
	}
	comments, err = second.ListComments(ctx, gameID)
	if err != nil || len(comments) != 1 || comments[0].ID != created.ID {
		t.Fatalf("second instance did not recover comment from backup: comments=%+v err=%v", comments, err)
	}
}

func TestPostgresStoreSharesCommentsAcrossInstances(t *testing.T) {
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
	gameID, userID, commentID, key := "game-"+suffix, "user-"+suffix, "comment-"+suffix, "key-"+suffix
	t.Cleanup(func() {
		_, _, _ = first.DeleteComment(context.Background(), gameID, commentID, userID)
		_, _ = first.db.ExecContext(context.Background(), "DELETE FROM social_comment_idempotency WHERE user_id = $1 AND idempotency_key = $2", userID, key)
		_ = first.Close()
		_ = second.Close()
	})
	comment, replay, err := first.CreateComment(ctx, Comment{ID: commentID, GameID: gameID, UserID: userID, Body: "hello"}, key)
	if err != nil || replay || comment.ID != commentID {
		t.Fatalf("create comment: comment=%+v replay=%v err=%v", comment, replay, err)
	}
	comment, replay, err = second.CreateComment(ctx, Comment{ID: "unused-id", GameID: gameID, UserID: userID, Body: "hello"}, key)
	if err != nil || !replay || comment.ID != commentID {
		t.Fatalf("idempotent retry: comment=%+v replay=%v err=%v", comment, replay, err)
	}
	_, _, err = second.CreateComment(ctx, Comment{ID: "different-id", GameID: gameID, UserID: userID, Body: "different body"}, key)
	if err != ErrCommentIdempotencyConflict {
		t.Fatalf("conflicting idempotency key error = %v", err)
	}
	comments, err := second.ListComments(ctx, gameID)
	if err != nil || len(comments) != 1 || comments[0].ID != commentID {
		t.Fatalf("cross-instance comments = %+v, err=%v", comments, err)
	}
	found, owned, err := second.DeleteComment(ctx, gameID, commentID, "not-the-author")
	if err != nil || !found || owned {
		t.Fatalf("non-author delete: found=%v owned=%v err=%v", found, owned, err)
	}
	found, owned, err = second.DeleteComment(ctx, gameID, commentID, userID)
	if err != nil || !found || !owned {
		t.Fatalf("author delete: found=%v owned=%v err=%v", found, owned, err)
	}
	comments, err = first.ListComments(ctx, gameID)
	if err != nil || len(comments) != 0 {
		t.Fatalf("cross-instance delete result: comments=%+v err=%v", comments, err)
	}
}
