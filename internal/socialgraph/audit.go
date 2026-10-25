package socialgraph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AuditEvent mirrors the file-backed moderationAuditEvent 1:1, plus a
// server-assigned seq that gives exact insertion order and a stable
// cursor (created_at ties are real, and ids are random).
type AuditEvent struct {
	ID        string
	Action    string
	GameID    string
	ReportID  string
	ActorID   string
	Status    string
	Reason    string
	CreatedAt time.Time
	Seq       int64
}

// AuditFilter is the list query for audit events. Cursor is the last
// seen seq; 0 means the first page. Limit 0 means the default of 100.
type AuditFilter struct {
	GameID   string
	ReportID string
	ActorID  string
	Action   string
	Limit    int
	Cursor   int64
}

// AuditStore is optional, like ModerationStore: file-backed development
// stores do not need a database. Production deployments should use it.
// Audit rows are append-only: there is no update or delete method, and
// a duplicate id is an error, never silently dropped.
type AuditStore interface {
	AppendAudit(ctx context.Context, ev AuditEvent) error
	ListAudit(ctx context.Context, f AuditFilter) ([]AuditEvent, string, error)
	CreateReportWithAudit(ctx context.Context, report Report, ev AuditEvent) error
	ResolveReportWithAudit(ctx context.Context, id, actorID, reason string, now time.Time, auditID string) (Report, bool, error)
}

func (s *PostgresStore) MigrateAudit(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS social_moderation_audit (
  id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 128),
  seq BIGSERIAL NOT NULL UNIQUE,
  action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  report_id TEXT NOT NULL DEFAULT '',
  actor_id TEXT NOT NULL CHECK (length(actor_id) BETWEEN 1 AND 128),
  status TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '' CHECK (length(reason) <= 2000),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS social_moderation_audit_game_created_idx
  ON social_moderation_audit (game_id, created_at DESC, seq DESC);
CREATE INDEX IF NOT EXISTS social_moderation_audit_action_created_idx
  ON social_moderation_audit (action, created_at DESC, seq DESC);
CREATE INDEX IF NOT EXISTS social_moderation_audit_actor_created_idx
  ON social_moderation_audit (actor_id, created_at DESC, seq DESC);
CREATE INDEX IF NOT EXISTS social_moderation_audit_report_created_idx
  ON social_moderation_audit (report_id, created_at DESC, seq DESC);`)
	if err != nil {
		return fmt.Errorf("migrate moderation audit: %w", err)
	}
	return nil
}

func validateAuditEvent(ev AuditEvent) error {
	if strings.TrimSpace(ev.ID) == "" || len(ev.ID) > 128 ||
		strings.TrimSpace(ev.Action) == "" || len(ev.Action) > 128 ||
		strings.TrimSpace(ev.GameID) == "" || len(ev.GameID) > 128 ||
		len(ev.ReportID) > 128 || len(ev.ActorID) > 128 || len(ev.Reason) > 2000 {
		return errors.New("audit event IDs, action and game must be 1-128 characters; reason at most 2000 characters")
	}
	if strings.TrimSpace(ev.ActorID) == "" {
		return errors.New("audit event actor must not be empty")
	}
	return nil
}

func (s *PostgresStore) AppendAudit(ctx context.Context, ev AuditEvent) error {
	ev.ID = strings.TrimSpace(ev.ID)
	ev.Action = strings.TrimSpace(ev.Action)
	ev.GameID = strings.TrimSpace(ev.GameID)
	ev.ReportID = strings.TrimSpace(ev.ReportID)
	ev.ActorID = strings.TrimSpace(ev.ActorID)
	ev.Status = strings.TrimSpace(ev.Status)
	ev.Reason = strings.TrimSpace(ev.Reason)
	if ev.ActorID == "" {
		ev.ActorID = "moderation-token"
	}
	if err := validateAuditEvent(ev); err != nil {
		return err
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO social_moderation_audit
(id, action, game_id, report_id, actor_id, status, reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ev.ID, ev.Action, ev.GameID, ev.ReportID, ev.ActorID, ev.Status, ev.Reason, ev.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert moderation audit: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEvent, string, error) {
	limit := f.Limit
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 || len(f.GameID) > 128 || len(f.ReportID) > 128 || len(f.ActorID) > 128 || len(f.Action) > 128 {
		return nil, "", errors.New("invalid audit filter or limit")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, seq, action, game_id, report_id, actor_id, status, reason, created_at
FROM social_moderation_audit
WHERE ($1 = '' OR game_id = $1) AND ($2 = '' OR report_id = $2)
  AND ($3 = '' OR actor_id = $3) AND ($4 = '' OR action = $4)
  AND ($5 = 0 OR seq < $5)
ORDER BY created_at DESC, seq DESC
LIMIT $6`, strings.TrimSpace(f.GameID), strings.TrimSpace(f.ReportID), strings.TrimSpace(f.ActorID), strings.TrimSpace(f.Action), f.Cursor, limit+1)
	if err != nil {
		return nil, "", fmt.Errorf("query moderation audit: %w", err)
	}
	defer rows.Close()
	events := make([]AuditEvent, 0)
	for rows.Next() {
		var ev AuditEvent
		if err := rows.Scan(&ev.ID, &ev.Seq, &ev.Action, &ev.GameID, &ev.ReportID, &ev.ActorID, &ev.Status, &ev.Reason, &ev.CreatedAt); err != nil {
			return nil, "", fmt.Errorf("scan moderation audit: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate moderation audit: %w", err)
	}
	nextCursor := ""
	if len(events) > limit {
		events = events[:limit]
		nextCursor = strconv.FormatInt(events[len(events)-1].Seq, 10)
	}
	return events, nextCursor, nil
}

// CreateReportWithAudit inserts a report and its audit row in one
// transaction, report first so a duplicate report id rolls back with no
// orphan audit row.
func (s *PostgresStore) CreateReportWithAudit(ctx context.Context, report Report, ev AuditEvent) error {
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
	ev.Action = strings.TrimSpace(ev.Action)
	if ev.Action == "" {
		ev.Action = "report_created"
	}
	ev.GameID = report.GameID
	ev.ReportID = report.ID
	if ev.ActorID == "" {
		ev.ActorID = report.ReporterID
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = report.CreatedAt
	}
	if err := validateAuditEvent(ev); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin report with audit: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
INSERT INTO social_moderation_reports
(id, game_id, reporter_id, reason, created_at, status)
VALUES ($1, $2, $3, $4, $5, 'open')
ON CONFLICT (id) DO NOTHING`,
		report.ID, report.GameID, report.ReporterID, report.Reason, report.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert moderation report with audit: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return errors.New("moderation report already exists")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO social_moderation_audit
(id, action, game_id, report_id, actor_id, status, reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ev.ID, ev.Action, ev.GameID, ev.ReportID, ev.ActorID, ev.Status, ev.Reason, ev.CreatedAt); err != nil {
		return fmt.Errorf("insert moderation audit with report: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit report with audit: %w", err)
	}
	return nil
}

// ResolveReportWithAudit resolves a report and audits the outcome in one
// transaction. If the report is already resolved, the report row is left
// untouched (first resolution preserved) and the audit action is
// report_resolve_attempted instead of report_resolved.
func (s *PostgresStore) ResolveReportWithAudit(ctx context.Context, id, actorID, reason string, now time.Time, auditID string) (Report, bool, error) {
	id, actorID, reason = strings.TrimSpace(id), strings.TrimSpace(actorID), strings.TrimSpace(reason)
	if id == "" || len(id) > 128 || actorID == "" || len(actorID) > 128 || len(reason) > 500 || strings.TrimSpace(auditID) == "" || len(auditID) > 128 {
		return Report{}, false, errors.New("report and actor IDs must be 1–128 characters; resolution reason must be at most 500 characters")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, false, fmt.Errorf("begin report resolution with audit: %w", err)
	}
	defer tx.Rollback()
	report, err := scanReport(tx.QueryRowContext(ctx, `
SELECT id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason
FROM social_moderation_reports WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, fmt.Errorf("read moderation report with audit: %w", err)
	}
	changed := false
	action := "report_resolve_attempted"
	if report.Status != "resolved" {
		if _, err := tx.ExecContext(ctx, `
UPDATE social_moderation_reports
SET status = 'resolved', resolved_at = $2, resolved_by = $3, resolution_reason = $4
WHERE id = $1`, id, now, actorID, reason); err != nil {
			return Report{}, false, fmt.Errorf("resolve moderation report with audit: %w", err)
		}
		report.Status, report.ResolvedAt, report.ResolvedBy, report.ResolutionReason = "resolved", &now, actorID, reason
		changed = true
		action = "report_resolved"
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO social_moderation_audit
(id, action, game_id, report_id, actor_id, status, reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		strings.TrimSpace(auditID), action, report.GameID, id, actorID, report.Status, reason, now); err != nil {
		return Report{}, false, fmt.Errorf("insert moderation audit with resolution: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Report{}, false, fmt.Errorf("commit report resolution with audit: %w", err)
	}
	return report, changed, nil
}
