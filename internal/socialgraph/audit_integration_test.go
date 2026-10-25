package socialgraph

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPostgresAuditAppendListAndFirstResolution(t *testing.T) {
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
	if err := graph.MigrateAudit(ctx); err != nil {
		t.Fatalf("migrate audit twice must be idempotent: %v", err)
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	gameID := "integration-audit-game-" + suffix
	reportID := "integration-audit-report-" + suffix
	viewer := "integration-audit-viewer-" + suffix

	// Append + list, with filters.
	if err := graph.AppendAudit(ctx, AuditEvent{
		ID: "integration-audit-ev-" + suffix, Action: "game_quarantined", GameID: gameID,
		ActorID: "moderation-token", Status: "quarantined", Reason: "manual quarantine", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}
	events, next, err := graph.ListAudit(ctx, AuditFilter{GameID: gameID})
	if err != nil || len(events) != 1 || events[0].Action != "game_quarantined" || next != "" {
		t.Fatalf("list audit by game = %+v next=%q err=%v, want one game_quarantined event", events, next, err)
	}
	// Duplicate audit id must error, not silently drop.
	if err := graph.AppendAudit(ctx, AuditEvent{
		ID: "integration-audit-ev-" + suffix, Action: "game_quarantined", GameID: gameID,
		ActorID: "moderation-token", CreatedAt: time.Now().UTC(),
	}); err == nil {
		t.Fatalf("duplicate audit id: got no error, want an error")
	}

	// Report with audit, then resolve twice: first resolution preserved,
	// and the second attempt is audited as an attempt, not a resolution.
	report := Report{ID: reportID, GameID: gameID, ReporterID: viewer, Reason: "audit test", CreatedAt: time.Now().UTC(), Status: "open"}
	if err := graph.CreateReportWithAudit(ctx, report, AuditEvent{
		ID: "integration-audit-create-" + suffix, Action: "report_created", GameID: gameID,
		ReportID: reportID, ActorID: viewer, Reason: "audit test", CreatedAt: report.CreatedAt,
	}); err != nil {
		t.Fatalf("create report with audit: %v", err)
	}
	now := time.Now().UTC()
	resolved, changed, err := graph.ResolveReportWithAudit(ctx, reportID, "moderator-1", "reviewed", now, "integration-audit-resolve-"+suffix)
	if err != nil || !changed || resolved.ResolvedBy != "moderator-1" {
		t.Fatalf("first resolve = %+v changed=%v err=%v, want moderator-1", resolved, changed, err)
	}
	resolved2, changed2, err := graph.ResolveReportWithAudit(ctx, reportID, "moderator-2", "different", now.Add(time.Second), "integration-audit-attempt-"+suffix)
	if err != nil || changed2 || resolved2.ResolvedBy != "moderator-1" || resolved2.ResolutionReason != "reviewed" {
		t.Fatalf("second resolve = %+v changed=%v err=%v, want first decision preserved", resolved2, changed2, err)
	}
	byReport, _, err := graph.ListAudit(ctx, AuditFilter{ReportID: reportID, Limit: 500})
	if err != nil {
		t.Fatalf("list audit by report: %v", err)
	}
	var resolvedCount, attemptCount int
	for _, ev := range byReport {
		if ev.Action == "report_resolved" {
			resolvedCount++
		}
		if ev.Action == "report_resolve_attempted" {
			attemptCount++
		}
	}
	if resolvedCount != 1 || attemptCount != 1 {
		t.Fatalf("audit actions for report: resolved=%d attempted=%d, want 1/1 (events=%+v)", resolvedCount, attemptCount, byReport)
	}
}
