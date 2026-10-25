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
		_ = graph.ResetPreferences(context.Background(), viewer)
		_ = graph.DeleteReaction(context.Background(), viewer, gameID)
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
	if err := graph.MarkNotInterested(ctx, viewer, gameID); err != nil {
		t.Fatalf("create not-interested relation: %v", err)
	}
	if err := graph.SetMutedTopics(ctx, viewer, []string{"backup-topic"}); err != nil {
		t.Fatalf("create muted topic: %v", err)
	}
	if err := graph.SetReaction(ctx, viewer, gameID, "like"); err != nil {
		t.Fatalf("create reaction: %v", err)
	}
	report := Report{ID: "integration-backup-report-" + time.Now().UTC().Format("20060102150405.000000000"), GameID: gameID, ReporterID: viewer, Reason: "backup test", CreatedAt: time.Now().UTC(), Status: "open"}
	if err := graph.CreateReport(ctx, report); err != nil {
		t.Fatalf("create moderation report: %v", err)
	}
	if _, _, err := graph.ResolveReport(ctx, report.ID, "moderator-1", "reviewed", time.Now().UTC()); err != nil {
		t.Fatalf("resolve moderation report before backup: %v", err)
	}
	auditID := "integration-backup-audit-" + time.Now().UTC().Format("20060102150405.000000000")
	if err := graph.AppendAudit(ctx, AuditEvent{ID: auditID, Action: "game_quarantined", GameID: gameID, ActorID: "moderation-token", Status: "quarantined", Reason: "backup test", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create audit event before backup: %v", err)
	}
	if err := graph.BackupTo(ctx, backupPath); err != nil {
		t.Fatalf("backup: %v", err)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil || !strings.Contains(string(backupData), auditID) {
		t.Fatalf("backup file contains audit id: err=%v, want %s in backup", err, auditID)
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
	if err := graph.ResetPreferences(ctx, viewer); err != nil {
		t.Fatalf("remove preferences before restore: %v", err)
	}
	if err := graph.DeleteReaction(ctx, viewer, gameID); err != nil {
		t.Fatalf("remove reaction before restore: %v", err)
	}
	if _, changed, err := graph.ResolveReport(ctx, report.ID, "moderator-2", "different reason", time.Now().UTC()); err != nil || changed {
		t.Fatalf("repeat resolution before restore changed first decision: changed=%v err=%v", changed, err)
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
	notInterested, err := graph.ListNotInterested(ctx, viewer)
	if err != nil || len(notInterested) != 1 || notInterested[0] != gameID {
		t.Fatalf("restored not-interested=%v err=%v, want [%s]", notInterested, err, gameID)
	}
	topics, err := graph.ListMutedTopics(ctx, viewer)
	if err != nil || len(topics) != 1 || topics[0] != "backup-topic" {
		t.Fatalf("restored muted topics=%v err=%v, want [backup-topic]", topics, err)
	}
	counts, viewerReactions, err := graph.ListReactionSummary(ctx, viewer, []string{gameID})
	if err != nil || counts[gameID] != 1 || viewerReactions[gameID] != "like" {
		t.Fatalf("restored reactions counts=%v viewer=%v err=%v, want count 1 and like", counts, viewerReactions, err)
	}
	auditEvents, _, err := graph.ListAudit(ctx, AuditFilter{GameID: gameID, Limit: 500})
	if err != nil {
		t.Fatalf("list audit after restore: %v", err)
	}
	var sawAudit bool
	for _, ev := range auditEvents {
		if ev.ID == auditID {
			sawAudit = true
		}
	}
	if !sawAudit {
		t.Fatalf("restored audit events=%+v, want event %s", auditEvents, auditID)
	}
	gotReport, found, err := graph.GetReport(ctx, report.ID)
	if err != nil || !found || gotReport.Status != "resolved" || gotReport.ResolvedBy != "moderator-1" || gotReport.ResolutionReason != "reviewed" {
		t.Fatalf("restored report=%+v found=%v err=%v, want resolved by moderator-1 with reviewed reason", gotReport, found, err)
	}
}
