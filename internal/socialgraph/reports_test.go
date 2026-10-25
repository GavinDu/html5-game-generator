package socialgraph

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestModerationReportValidationRunsBeforeDatabaseAccess(t *testing.T) {
	s := &PostgresStore{}
	if err := s.CreateReport(context.Background(), Report{ID: "", GameID: "game", ReporterID: "user", Reason: "valid"}); err == nil {
		t.Fatal("empty report ID should be rejected before database access")
	}
	if err := s.CreateReport(context.Background(), Report{ID: "report", GameID: "game", ReporterID: "user", Reason: strings.Repeat("r", 501)}); err == nil {
		t.Fatal("oversized report reason should be rejected before database access")
	}
	if _, found, err := s.GetReport(context.Background(), ""); err == nil || found {
		t.Fatalf("empty report lookup = found %v err %v", found, err)
	}
	if _, _, err := s.ResolveReport(context.Background(), "", "moderator", "", time.Now()); err == nil {
		t.Fatal("empty report ID should be rejected before database access")
	}
	if _, err := s.ListReports(context.Background(), "unknown", "", 10); err == nil {
		t.Fatal("unknown report status should be rejected before database access")
	}
	if _, err := s.ListReports(context.Background(), "", "", 501); err == nil {
		t.Fatal("oversized report limit should be rejected before database access")
	}
}
