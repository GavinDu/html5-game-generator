package socialgraph

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store-level cursor pagination (step 1 of the #63 cursor contract).
// These methods are additive: every existing unpaged List* method stays
// untouched, so no handler breaks. Handlers migrate one endpoint at a
// time in follow-up PRs. Internal filter call sites (feed visibility,
// block checks) must keep using the full-list methods, not these pages,
// or a filter set would be silently truncated.

func (s *PostgresStore) idPage(ctx context.Context, scope, query, filterKind string, p Page) (IDPage, error) {
	if err := validateID(scope); err != nil {
		return IDPage{}, err
	}
	limit, err := resolveIDPageLimit(p.Limit)
	if err != nil {
		return IDPage{}, err
	}
	filter := filterKind + ":" + scope
	cursorKey := ""
	if strings.TrimSpace(p.Cursor) != "" {
		keys, err := decodeCursor(p.Cursor, filter)
		if err != nil {
			return IDPage{}, err
		}
		if len(keys) != 1 {
			return IDPage{}, fmt.Errorf("cursor has wrong key arity for an id list")
		}
		cursorKey = keys[0]
	}
	rows, err := s.db.QueryContext(ctx, query, scope, cursorKey, limit+1)
	if err != nil {
		return IDPage{}, fmt.Errorf("query paged ids: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return IDPage{}, fmt.Errorf("scan paged id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return IDPage{}, fmt.Errorf("iterate paged ids: %w", err)
	}
	page := IDPage{IDs: ids}
	if len(ids) > limit {
		page.IDs = ids[:limit]
		next, err := encodeCursor([]string{page.IDs[len(page.IDs)-1]}, filter)
		if err != nil {
			return IDPage{}, err
		}
		page.NextCursor = next
	}
	return page, nil
}

func (s *PostgresStore) ListFollowingPage(ctx context.Context, viewerID string, p Page) (IDPage, error) {
	return s.idPage(ctx, viewerID, `SELECT creator_id FROM social_follows WHERE viewer_id = $1 AND creator_id > $2 ORDER BY creator_id LIMIT $3`, "following", p)
}

func (s *PostgresStore) ListFollowersPage(ctx context.Context, creatorID string, p Page) (IDPage, error) {
	return s.idPage(ctx, creatorID, `SELECT viewer_id FROM social_follows WHERE creator_id = $1 AND viewer_id > $2 ORDER BY viewer_id LIMIT $3`, "followers", p)
}

func (s *PostgresStore) ListBlockedPage(ctx context.Context, viewerID string, p Page) (IDPage, error) {
	return s.idPage(ctx, viewerID, `SELECT blocked_id FROM social_blocks WHERE viewer_id = $1 AND blocked_id > $2 ORDER BY blocked_id LIMIT $3`, "blocked", p)
}

func (s *PostgresStore) ListBlockersPage(ctx context.Context, blockedID string, p Page) (IDPage, error) {
	return s.idPage(ctx, blockedID, `SELECT viewer_id FROM social_blocks WHERE blocked_id = $1 AND viewer_id > $2 ORDER BY viewer_id LIMIT $3`, "blockers", p)
}

func (s *PostgresStore) ListSavedGamesPage(ctx context.Context, viewerID string, p Page) (IDPage, error) {
	return s.idPage(ctx, viewerID, `SELECT game_id FROM social_saves WHERE viewer_id = $1 AND game_id > $2 ORDER BY game_id LIMIT $3`, "saved", p)
}

func (s *PostgresStore) ListNotInterestedPage(ctx context.Context, viewerID string, p Page) (IDPage, error) {
	return s.idPage(ctx, viewerID, `SELECT game_id FROM social_not_interested WHERE viewer_id = $1 AND game_id > $2 ORDER BY game_id LIMIT $3`, "not-interested", p)
}

func (s *PostgresStore) ListMutedTopicsPage(ctx context.Context, viewerID string, p Page) (IDPage, error) {
	return s.idPage(ctx, viewerID, `SELECT topic FROM social_muted_topics WHERE viewer_id = $1 AND topic > $2 ORDER BY topic LIMIT $3`, "muted-topics", p)
}

func (s *PostgresStore) ListCommentsPage(ctx context.Context, gameID string, p Page) (CommentPage, error) {
	if err := validateID(gameID); err != nil {
		return CommentPage{}, err
	}
	limit, err := resolveIDPageLimit(p.Limit)
	if err != nil {
		return CommentPage{}, err
	}
	filter := "comments:" + gameID
	var rows *sql.Rows
	if strings.TrimSpace(p.Cursor) == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT id, game_id, user_id, body, created_at FROM social_comments WHERE game_id = $1 ORDER BY created_at, id LIMIT $2`, gameID, limit+1)
	} else {
		keys, err2 := decodeCursor(p.Cursor, filter)
		if err2 != nil {
			return CommentPage{}, err2
		}
		if len(keys) != 2 {
			return CommentPage{}, fmt.Errorf("cursor has wrong key arity for comments")
		}
		ts, err2 := time.Parse(time.RFC3339Nano, keys[0])
		if err2 != nil {
			return CommentPage{}, fmt.Errorf("cursor timestamp is invalid")
		}
		rows, err = s.db.QueryContext(ctx, `SELECT id, game_id, user_id, body, created_at FROM social_comments WHERE game_id = $1 AND (created_at, id) > ($2, $3) ORDER BY created_at, id LIMIT $4`, gameID, ts, keys[1], limit+1)
	}
	if err != nil {
		return CommentPage{}, fmt.Errorf("query paged comments: %w", err)
	}
	defer rows.Close()
	comments := make([]Comment, 0)
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.GameID, &comment.UserID, &comment.Body, &comment.CreatedAt); err != nil {
			return CommentPage{}, fmt.Errorf("scan paged comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return CommentPage{}, fmt.Errorf("iterate paged comments: %w", err)
	}
	page := CommentPage{Comments: comments}
	if len(comments) > limit {
		page.Comments = comments[:limit]
		last := page.Comments[len(page.Comments)-1]
		next, err := encodeCursor([]string{last.CreatedAt.Format(time.RFC3339Nano), last.ID}, filter)
		if err != nil {
			return CommentPage{}, err
		}
		page.NextCursor = next
	}
	return page, nil
}

func (s *PostgresStore) ListReportsPage(ctx context.Context, status, gameID string, p Page) (ReportPage, error) {
	status, gameID = strings.TrimSpace(status), strings.TrimSpace(gameID)
	if (status != "" && status != "open" && status != "resolved") || len(gameID) > 128 {
		return ReportPage{}, fmt.Errorf("invalid report filter")
	}
	limit, err := resolveReportPageLimit(p.Limit)
	if err != nil {
		return ReportPage{}, err
	}
	filter := "reports:" + status + "|" + gameID
	// Build the WHERE clause with AND conditions only (never the OR
	// form), so the status/game indexes stay usable.
	query := `SELECT id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason FROM social_moderation_reports WHERE 1=1`
	args := make([]any, 0)
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if gameID != "" {
		args = append(args, gameID)
		query += fmt.Sprintf(" AND game_id = $%d", len(args))
	}
	if strings.TrimSpace(p.Cursor) != "" {
		keys, err := decodeCursor(p.Cursor, filter)
		if err != nil {
			return ReportPage{}, err
		}
		if len(keys) != 2 {
			return ReportPage{}, fmt.Errorf("cursor has wrong key arity for reports")
		}
		ts, err := time.Parse(time.RFC3339Nano, keys[0])
		if err != nil {
			return ReportPage{}, fmt.Errorf("cursor timestamp is invalid")
		}
		args = append(args, ts, keys[1])
		query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit+1)
	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ReportPage{}, fmt.Errorf("query paged reports: %w", err)
	}
	defer rows.Close()
	reports := make([]Report, 0)
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return ReportPage{}, fmt.Errorf("scan paged report: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return ReportPage{}, fmt.Errorf("iterate paged reports: %w", err)
	}
	page := ReportPage{Reports: reports}
	if len(reports) > limit {
		page.Reports = reports[:limit]
		last := page.Reports[len(page.Reports)-1]
		next, err := encodeCursor([]string{last.CreatedAt.Format(time.RFC3339Nano), last.ID}, filter)
		if err != nil {
			return ReportPage{}, err
		}
		page.NextCursor = next
	}
	return page, nil
}
