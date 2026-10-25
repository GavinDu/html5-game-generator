package socialgraph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadPostgresBackupRejectsChecksumMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.json")
	envelope := postgresBackupEnvelope{
		FormatVersion: postgresBackupFormatVersion,
		SHA256:        strings.Repeat("0", 64),
		Snapshot:      json.RawMessage(`{"follows":[],"blocks":[],"saves":[]}`),
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readPostgresBackup(path); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("read corrupted backup error = %v, want checksum mismatch", err)
	}
}

func TestPostgresBackupRestoreRoundTrip(t *testing.T) {
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
	viewer := "integration-backup-viewer-" + time.Now().UTC().Format("20060102150405.000000000")
	creator := "integration-backup-creator-" + time.Now().UTC().Format("20060102150405.000000000")
	blocked := "integration-backup-blocked-" + time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-backup-saved-game-" + time.Now().UTC().Format("20060102150405.000000000")
	backupPath := filepath.Join(t.TempDir(), "social-graph.json")
	t.Cleanup(func() {
		_ = graph.Unfollow(context.Background(), viewer, creator)
		_ = graph.Unblock(context.Background(), viewer, blocked)
		_ = graph.UnsaveGame(context.Background(), viewer, gameID)
		_ = graph.Close()
	})
	if err := graph.Follow(ctx, viewer, creator); err != nil {
		t.Fatalf("create follow relation: %v", err)
	}
	if err := graph.Block(ctx, viewer, blocked); err != nil {
		t.Fatalf("create block relation: %v", err)
	}
	if err := graph.SaveGame(ctx, viewer, gameID); err != nil {
		t.Fatalf("create saved-game relation: %v", err)
	}
	if err := graph.BackupTo(ctx, backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := graph.Unfollow(ctx, viewer, creator); err != nil {
		t.Fatalf("remove follow before restore: %v", err)
	}
	if err := graph.Unblock(ctx, viewer, blocked); err != nil {
		t.Fatalf("remove block before restore: %v", err)
	}
	if err := graph.UnsaveGame(ctx, viewer, gameID); err != nil {
		t.Fatalf("remove save before restore: %v", err)
	}
	if err := graph.RestoreFrom(ctx, backupPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	following, err := graph.ListFollowing(ctx, viewer)
	if err != nil || len(following) != 1 || following[0] != creator {
		t.Fatalf("restored follows=%v err=%v, want [%s]", following, err, creator)
	}
	blocks, err := graph.ListBlocked(ctx, viewer)
	if err != nil || len(blocks) != 1 || blocks[0] != blocked {
		t.Fatalf("restored blocks=%v err=%v, want [%s]", blocks, err, blocked)
	}
	saves, err := graph.ListSavedGames(ctx, viewer)
	if err != nil || len(saves) != 1 || saves[0] != gameID {
		t.Fatalf("restored saves=%v err=%v, want [%s]", saves, err, gameID)
	}
}
