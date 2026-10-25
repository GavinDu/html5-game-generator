package socialgraph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Report is a durable moderation report and its first resolution decision.
type Report struct {
	ID               string
	GameID           string
	ReporterID       string
	Reason           string
	CreatedAt        time.Time
	Status           string
	ResolvedAt       *time.Time
	ResolvedBy       string
	ResolutionReason string
}

// ModerationStore is optional so local/file-backed development stores do not
// need a database. Production deployments should use it for durable reports.
type ModerationStore interface {
	CreateReport(context.Context, Report) error
	ResolveReport(context.Context, string, string, string, time.Time) (Report, bool, error)
	ListReports(context.Context, string, string, int) ([]Report, error)
}

func (s *PostgresStore) MigrateModeration(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS social_moderation_reports (
  id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  reporter_id TEXT NOT NULL CHECK (length(reporter_id) BETWEEN 1 AND 128),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
  created_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('open', 'resolved')),
  resolved_at TIMESTAMPTZ,
  resolved_by TEXT NOT NULL DEFAULT '',
  resolution_reason TEXT NOT NULL DEFAULT '' CHECK (length(resolution_reason) <= 500)
);
CREATE INDEX IF NOT EXISTS social_moderation_reports_status_created_idx
  ON social_moderation_reports (status, created_at DESC);
CREATE INDEX IF NOT EXISTS social_moderation_reports_game_created_idx
  ON social_moderation_reports (game_id, created_at DESC);`)
	if err != nil {
		return fmt.Errorf("migrate moderation reports: %w", err)
	}
	return nil
}

func (s *PostgresStore) CreateReport(ctx context.Context, report Report) error {
	report.ID = strings.TrimSpace(report.ID)
	report.GameID = strings.TrimSpace(report.GameID)
	report.ReporterID = strings.TrimSpace(report.ReporterID)
	report.Reason = strings.TrimSpace(report.Reason)
	if report.ID == "" || report.GameID == "" || report.ReporterID == "" ||
		len(report.ID) > 128 || len(report.GameID) > 128 || len(report.ReporterID) > 128 ||
		report.Reason == "" || len(report.Reason) > 500 {
		return errors.New("report IDs must be 1–128 characters and reason 1–500 characters")
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	if report.Status == "" {
		report.Status = "open"
	}
	if report.Status != "open" {
		return errors.New("new moderation report must have open status")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO social_moderation_reports
(id, game_id, reporter_id, reason, created_at, status)
VALUES ($1, $2, $3, $4, $5, 'open')
ON CONFLICT (id) DO NOTHING`,
		report.ID, report.GameID, report.ReporterID, report.Reason, report.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert moderation report: %w", err)
	}
	return nil
}

func (s *PostgresStore) ResolveReport(ctx context.Context, id, actorID, reason string, now time.Time) (Report, bool, error) {
	id, actorID, reason = strings.TrimSpace(id), strings.TrimSpace(actorID), strings.TrimSpace(reason)
	if id == "" || len(id) > 128 || actorID == "" || len(actorID) > 128 || len(reason) > 500 {
		return Report{}, false, errors.New("report and actor IDs must be 1–128 characters; resolution reason must be at most 500 characters")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, false, fmt.Errorf("begin report resolution: %w", err)
	}
	defer tx.Rollback()
	report, err := scanReport(tx.QueryRowContext(ctx, `
SELECT id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason
FROM social_moderation_reports WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, fmt.Errorf("read moderation report: %w", err)
	}
	changed := false
	if report.Status != "resolved" {
		_, err = tx.ExecContext(ctx, `
UPDATE social_moderation_reports
SET status = 'resolved', resolved_at = $2, resolved_by = $3, resolution_reason = $4
WHERE id = $1`, id, now, actorID, reason)
		if err != nil {
			return Report{}, false, fmt.Errorf("resolve moderation report: %w", err)
		}
		report.Status, report.ResolvedAt, report.ResolvedBy, report.ResolutionReason = "resolved", &now, actorID, reason
		changed = true
	}
	if err := tx.Commit(); err != nil {
		return Report{}, false, fmt.Errorf("commit moderation report resolution: %w", err)
	}
	return report, changed, nil
}

type reportScanner interface {
	Scan(...any) error
}

func scanReport(row reportScanner) (Report, error) {
	var report Report
	err := row.Scan(&report.ID, &report.GameID, &report.ReporterID, &report.Reason, &report.CreatedAt,
		&report.Status, &report.ResolvedAt, &report.ResolvedBy, &report.ResolutionReason)
	return report, err
}

func (s *PostgresStore) ListReports(ctx context.Context, status, gameID string, limit int) ([]Report, error) {
	status, gameID = strings.TrimSpace(status), strings.TrimSpace(gameID)
	if (status != "" && status != "open" && status != "resolved") || len(gameID) > 128 || limit < 1 || limit > 500 {
		return nil, errors.New("invalid report filter or limit")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason
FROM social_moderation_reports
WHERE ($1 = '' OR status = $1) AND ($2 = '' OR game_id = $2)
ORDER BY created_at DESC, id DESC
LIMIT $3`, status, gameID, limit)
	if err != nil {
		return nil, fmt.Errorf("query moderation reports: %w", err)
	}
	defer rows.Close()
	reports := make([]Report, 0)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, fmt.Errorf("scan moderation report: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation reports: %w", err)
	}
	return reports, nil
}
