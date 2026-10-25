package socialgraph

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const postgresBackupFormatVersion = 3

type postgresBackupEnvelope struct {
	FormatVersion int             `json:"formatVersion"`
	SHA256        string          `json:"sha256"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

type postgresBackupSnapshot struct {
	Follows            []Edge                       `json:"follows"`
	Blocks             []BlockRelation              `json:"blocks"`
	Saves              []SaveRelation               `json:"saves"`
	Comments           []Comment                    `json:"comments"`
	CommentIdempotency []CommentIdempotencyRelation `json:"commentIdempotency"`
	NotInterested      []NotInterestedRelation      `json:"notInterested"`
	MutedTopics        []MutedTopicRelation         `json:"mutedTopics"`
	Reactions          []ReactionRelation           `json:"reactions"`
	Reports            []Report                     `json:"reports"`
}

type NotInterestedRelation struct {
	ViewerID string
	GameID   string
}

type MutedTopicRelation struct {
	ViewerID string
	Topic    string
}

type ReactionRelation struct {
	ViewerID  string
	GameID    string
	Reaction  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BackupTo exports a consistent, checksummed snapshot of all shared social
// graph records: follow, block, saved-game, comment, comment-idempotency,
// not-interested, muted-topic, reaction, and moderation-report records.
// Format version 3 is the first version that includes the last four
// collections; version 1 and 2 snapshots are still readable, with the
// collections they predate treated as empty. The backup is written with
// restrictive permissions and an atomic rename so incomplete files are
// not mistaken for valid backups.
func (s *PostgresStore) BackupTo(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin social graph backup: %w", err)
	}
	defer tx.Rollback()
	snapshot := postgresBackupSnapshot{
		Follows:            make([]Edge, 0),
		Blocks:             make([]BlockRelation, 0),
		Saves:              make([]SaveRelation, 0),
		Comments:           make([]Comment, 0),
		CommentIdempotency: make([]CommentIdempotencyRelation, 0),
		NotInterested:      make([]NotInterestedRelation, 0),
		MutedTopics:        make([]MutedTopicRelation, 0),
		Reactions:          make([]ReactionRelation, 0),
		Reports:            make([]Report, 0),
	}
	rows, err := tx.QueryContext(ctx, `SELECT viewer_id, creator_id FROM social_follows ORDER BY viewer_id, creator_id`)
	if err != nil {
		return fmt.Errorf("query follow relations for backup: %w", err)
	}
	for rows.Next() {
		var edge Edge
		if err := rows.Scan(&edge.ViewerID, &edge.CreatorID); err != nil {
			rows.Close()
			return fmt.Errorf("scan follow relation for backup: %w", err)
		}
		snapshot.Follows = append(snapshot.Follows, edge)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate follow relations for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close follow relation rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT viewer_id, blocked_id FROM social_blocks ORDER BY viewer_id, blocked_id`)
	if err != nil {
		return fmt.Errorf("query block relations for backup: %w", err)
	}
	for rows.Next() {
		var relation BlockRelation
		if err := rows.Scan(&relation.ViewerID, &relation.BlockedID); err != nil {
			rows.Close()
			return fmt.Errorf("scan block relation for backup: %w", err)
		}
		snapshot.Blocks = append(snapshot.Blocks, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate block relations for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close block relation rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT viewer_id, game_id FROM social_saves ORDER BY viewer_id, game_id`)
	if err != nil {
		return fmt.Errorf("query saved-game relations for backup: %w", err)
	}
	for rows.Next() {
		var relation SaveRelation
		if err := rows.Scan(&relation.ViewerID, &relation.GameID); err != nil {
			rows.Close()
			return fmt.Errorf("scan saved-game relation for backup: %w", err)
		}
		snapshot.Saves = append(snapshot.Saves, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate saved-game relations for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close saved-game relation rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, game_id, user_id, body, created_at FROM social_comments ORDER BY game_id, created_at, id`)
	if err != nil {
		return fmt.Errorf("query comments for backup: %w", err)
	}
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.GameID, &comment.UserID, &comment.Body, &comment.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan comment for backup: %w", err)
		}
		snapshot.Comments = append(snapshot.Comments, comment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate comments for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close comment rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT user_id, idempotency_key, game_id, body, comment_id, created_at FROM social_comment_idempotency ORDER BY idempotency_key`)
	if err != nil {
		return fmt.Errorf("query comment idempotency for backup: %w", err)
	}
	for rows.Next() {
		var relation CommentIdempotencyRelation
		if err := rows.Scan(&relation.UserID, &relation.Key, &relation.GameID, &relation.Body, &relation.CommentID, &relation.CreatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan comment idempotency for backup: %w", err)
		}
		snapshot.CommentIdempotency = append(snapshot.CommentIdempotency, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate comment idempotency for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close comment idempotency rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT viewer_id, game_id FROM social_not_interested ORDER BY viewer_id, game_id`)
	if err != nil {
		return fmt.Errorf("query not-interested relations for backup: %w", err)
	}
	for rows.Next() {
		var relation NotInterestedRelation
		if err := rows.Scan(&relation.ViewerID, &relation.GameID); err != nil {
			rows.Close()
			return fmt.Errorf("scan not-interested relation for backup: %w", err)
		}
		snapshot.NotInterested = append(snapshot.NotInterested, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate not-interested relations for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close not-interested relation rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT viewer_id, topic FROM social_muted_topics ORDER BY viewer_id, topic`)
	if err != nil {
		return fmt.Errorf("query muted topics for backup: %w", err)
	}
	for rows.Next() {
		var relation MutedTopicRelation
		if err := rows.Scan(&relation.ViewerID, &relation.Topic); err != nil {
			rows.Close()
			return fmt.Errorf("scan muted topic for backup: %w", err)
		}
		snapshot.MutedTopics = append(snapshot.MutedTopics, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate muted topics for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close muted topic rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT viewer_id, game_id, reaction, created_at, updated_at FROM social_reactions ORDER BY viewer_id, game_id`)
	if err != nil {
		return fmt.Errorf("query reactions for backup: %w", err)
	}
	for rows.Next() {
		var relation ReactionRelation
		if err := rows.Scan(&relation.ViewerID, &relation.GameID, &relation.Reaction, &relation.CreatedAt, &relation.UpdatedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan reaction for backup: %w", err)
		}
		snapshot.Reactions = append(snapshot.Reactions, relation)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate reactions for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close reaction rows: %w", err)
	}
	rows, err = tx.QueryContext(ctx, `SELECT id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason FROM social_moderation_reports ORDER BY created_at, id`)
	if err != nil {
		return fmt.Errorf("query moderation reports for backup: %w", err)
	}
	for rows.Next() {
		var report Report
		if err := rows.Scan(&report.ID, &report.GameID, &report.ReporterID, &report.Reason, &report.CreatedAt, &report.Status, &report.ResolvedAt, &report.ResolvedBy, &report.ResolutionReason); err != nil {
			rows.Close()
			return fmt.Errorf("scan moderation report for backup: %w", err)
		}
		snapshot.Reports = append(snapshot.Reports, report)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate moderation reports for backup: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close moderation report rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("finish social graph backup snapshot: %w", err)
	}
	rawSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode social graph backup: %w", err)
	}
	sum := sha256.Sum256(rawSnapshot)
	encoded, err := json.Marshal(postgresBackupEnvelope{
		FormatVersion: postgresBackupFormatVersion,
		SHA256:        hex.EncodeToString(sum[:]),
		Snapshot:      rawSnapshot,
	})
	if err != nil {
		return fmt.Errorf("encode social graph backup envelope: %w", err)
	}
	return writePostgresBackupAtomically(path, encoded)
}

// RestoreFrom validates a backup before writing any rows. Restore is additive:
// it is safe to retry and never deletes destination relations. For an exact
// point-in-time restore, use an empty target database and stop application writes.
func (s *PostgresStore) RestoreFrom(ctx context.Context, path string) error {
	snapshot, err := readPostgresBackup(path)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin social graph restore: %w", err)
	}
	defer tx.Rollback()
	followStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_follows (viewer_id, creator_id) VALUES ($1, $2) ON CONFLICT (viewer_id, creator_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare follow restore: %w", err)
	}
	defer followStmt.Close()
	blockStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_blocks (viewer_id, blocked_id) VALUES ($1, $2) ON CONFLICT (viewer_id, blocked_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare block restore: %w", err)
	}
	defer blockStmt.Close()
	saveStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_saves (viewer_id, game_id) VALUES ($1, $2) ON CONFLICT (viewer_id, game_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare saved-game restore: %w", err)
	}
	defer saveStmt.Close()
	for _, edge := range snapshot.Follows {
		if _, err := followStmt.ExecContext(ctx, edge.ViewerID, edge.CreatorID); err != nil {
			return fmt.Errorf("restore follow relation: %w", err)
		}
	}
	for _, relation := range snapshot.Blocks {
		if _, err := blockStmt.ExecContext(ctx, relation.ViewerID, relation.BlockedID); err != nil {
			return fmt.Errorf("restore block relation: %w", err)
		}
	}
	for _, relation := range snapshot.Saves {
		if _, err := saveStmt.ExecContext(ctx, relation.ViewerID, relation.GameID); err != nil {
			return fmt.Errorf("restore saved-game relation: %w", err)
		}
	}
	commentStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_comments (id, game_id, user_id, body, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare comment restore: %w", err)
	}
	defer commentStmt.Close()
	for _, comment := range snapshot.Comments {
		if _, err := commentStmt.ExecContext(ctx, comment.ID, comment.GameID, comment.UserID, comment.Body, comment.CreatedAt); err != nil {
			return fmt.Errorf("restore comment: %w", err)
		}
	}
	idempotencyStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_comment_idempotency (user_id, idempotency_key, game_id, body, comment_id, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (idempotency_key) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare comment idempotency restore: %w", err)
	}
	defer idempotencyStmt.Close()
	for _, relation := range snapshot.CommentIdempotency {
		if _, err := idempotencyStmt.ExecContext(ctx, relation.UserID, relation.Key, relation.GameID, relation.Body, relation.CommentID, relation.CreatedAt); err != nil {
			return fmt.Errorf("restore comment idempotency: %w", err)
		}
	}

	notInterestedStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_not_interested (viewer_id, game_id) VALUES ($1, $2) ON CONFLICT (viewer_id, game_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare not-interested restore: %w", err)
	}
	defer notInterestedStmt.Close()
	for _, relation := range snapshot.NotInterested {
		if _, err := notInterestedStmt.ExecContext(ctx, relation.ViewerID, relation.GameID); err != nil {
			return fmt.Errorf("restore not-interested relation: %w", err)
		}
	}
	mutedTopicStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_muted_topics (viewer_id, topic) VALUES ($1, $2) ON CONFLICT (viewer_id, topic) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare muted topic restore: %w", err)
	}
	defer mutedTopicStmt.Close()
	for _, relation := range snapshot.MutedTopics {
		if _, err := mutedTopicStmt.ExecContext(ctx, relation.ViewerID, relation.Topic); err != nil {
			return fmt.Errorf("restore muted topic: %w", err)
		}
	}
	reactionStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_reactions (viewer_id, game_id, reaction, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT (viewer_id, game_id) DO UPDATE SET reaction = EXCLUDED.reaction, updated_at = EXCLUDED.updated_at`)
	if err != nil {
		return fmt.Errorf("prepare reaction restore: %w", err)
	}
	defer reactionStmt.Close()
	for _, relation := range snapshot.Reactions {
		created, updated := relation.CreatedAt, relation.UpdatedAt
		if created.IsZero() || updated.IsZero() {
			now := time.Now().UTC()
			if created.IsZero() {
				created = now
			}
			if updated.IsZero() {
				updated = now
			}
		}
		if _, err := reactionStmt.ExecContext(ctx, relation.ViewerID, relation.GameID, relation.Reaction, created, updated); err != nil {
			return fmt.Errorf("restore reaction: %w", err)
		}
	}
	reportStmt, err := tx.PrepareContext(ctx, `INSERT INTO social_moderation_reports (id, game_id, reporter_id, reason, created_at, status, resolved_at, resolved_by, resolution_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare moderation report restore: %w", err)
	}
	defer reportStmt.Close()
	for _, report := range snapshot.Reports {
		if _, err := reportStmt.ExecContext(ctx, report.ID, report.GameID, report.ReporterID, report.Reason, report.CreatedAt, report.Status, report.ResolvedAt, report.ResolvedBy, report.ResolutionReason); err != nil {
			return fmt.Errorf("restore moderation report: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit social graph restore: %w", err)
	}
	return nil
}

func readPostgresBackup(path string) (postgresBackupSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return postgresBackupSnapshot{}, fmt.Errorf("read social graph backup: %w", err)
	}
	var envelope postgresBackupEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return postgresBackupSnapshot{}, fmt.Errorf("decode social graph backup envelope: %w", err)
	}
	if envelope.FormatVersion != 1 && envelope.FormatVersion != 2 && envelope.FormatVersion != postgresBackupFormatVersion {
		return postgresBackupSnapshot{}, fmt.Errorf("unsupported social graph backup format version %d", envelope.FormatVersion)
	}
	sum := sha256.Sum256(envelope.Snapshot)
	if envelope.SHA256 != hex.EncodeToString(sum[:]) {
		return postgresBackupSnapshot{}, fmt.Errorf("social graph backup checksum mismatch")
	}
	var snapshot postgresBackupSnapshot
	if err := json.Unmarshal(envelope.Snapshot, &snapshot); err != nil {
		return postgresBackupSnapshot{}, fmt.Errorf("decode social graph backup snapshot: %w", err)
	}
	if snapshot.Follows == nil || snapshot.Blocks == nil || snapshot.Saves == nil {
		return postgresBackupSnapshot{}, fmt.Errorf("social graph backup is missing required collections")
	}
	// Version 1 snapshots predate PostgreSQL comment storage, and versions
	// 1 and 2 predate not-interested, muted-topic, reaction, and report
	// storage. Those collections are treated as empty in old snapshots.
	// Version 3 snapshots must carry every collection, so a v3 file missing
	// one is corrupt rather than merely old.
	if envelope.FormatVersion == 1 {
		snapshot.Comments = []Comment{}
		snapshot.CommentIdempotency = []CommentIdempotencyRelation{}
		snapshot.NotInterested = []NotInterestedRelation{}
		snapshot.MutedTopics = []MutedTopicRelation{}
		snapshot.Reactions = []ReactionRelation{}
		snapshot.Reports = []Report{}
	} else if envelope.FormatVersion == 2 {
		if snapshot.Comments == nil || snapshot.CommentIdempotency == nil {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup is missing comment collections")
		}
		if snapshot.NotInterested == nil {
			snapshot.NotInterested = []NotInterestedRelation{}
		}
		if snapshot.MutedTopics == nil {
			snapshot.MutedTopics = []MutedTopicRelation{}
		}
		if snapshot.Reactions == nil {
			snapshot.Reactions = []ReactionRelation{}
		}
		if snapshot.Reports == nil {
			snapshot.Reports = []Report{}
		}
	} else if snapshot.Comments == nil || snapshot.CommentIdempotency == nil || snapshot.NotInterested == nil || snapshot.MutedTopics == nil || snapshot.Reactions == nil || snapshot.Reports == nil {
		return postgresBackupSnapshot{}, fmt.Errorf("social graph backup is missing required collections")
	}
	for _, edge := range snapshot.Follows {
		viewer, creator := strings.TrimSpace(edge.ViewerID), strings.TrimSpace(edge.CreatorID)
		if viewer == "" || creator == "" || len(viewer) > 128 || len(creator) > 128 || viewer == creator {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid follow relation")
		}
	}
	for _, relation := range snapshot.Blocks {
		viewer, blocked := strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.BlockedID)
		if viewer == "" || blocked == "" || len(viewer) > 128 || len(blocked) > 128 || viewer == blocked {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid block relation")
		}
	}
	for _, relation := range snapshot.Saves {
		viewer, gameID := strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.GameID)
		if viewer == "" || gameID == "" || len(viewer) > 128 || len(gameID) > 128 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid saved-game relation")
		}
	}
	for _, comment := range snapshot.Comments {
		if strings.TrimSpace(comment.ID) == "" || strings.TrimSpace(comment.GameID) == "" ||
			strings.TrimSpace(comment.UserID) == "" || strings.TrimSpace(comment.Body) == "" ||
			len(comment.ID) > 128 || len(comment.GameID) > 128 || len(comment.UserID) > 128 || len(comment.Body) > 2000 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid comment")
		}
	}
	for _, relation := range snapshot.CommentIdempotency {
		if strings.TrimSpace(relation.UserID) == "" || strings.TrimSpace(relation.Key) == "" ||
			strings.TrimSpace(relation.GameID) == "" || strings.TrimSpace(relation.Body) == "" ||
			strings.TrimSpace(relation.CommentID) == "" || len(relation.UserID) > 128 ||
			len(relation.Key) > 128 || len(relation.GameID) > 128 || len(relation.Body) > 2000 || len(relation.CommentID) > 128 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid comment idempotency record")
		}
	}

	for _, relation := range snapshot.NotInterested {
		if strings.TrimSpace(relation.ViewerID) == "" || strings.TrimSpace(relation.GameID) == "" || len(relation.ViewerID) > 128 || len(relation.GameID) > 128 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid not-interested relation")
		}
	}
	for _, relation := range snapshot.MutedTopics {
		if strings.TrimSpace(relation.ViewerID) == "" || strings.TrimSpace(relation.Topic) == "" || len(relation.ViewerID) > 128 || len(relation.Topic) > 40 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid muted topic")
		}
	}
	for _, relation := range snapshot.Reactions {
		switch relation.Reaction {
		case "like", "love", "funny", "wow":
		default:
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid reaction")
		}
		if strings.TrimSpace(relation.ViewerID) == "" || strings.TrimSpace(relation.GameID) == "" || len(relation.ViewerID) > 128 || len(relation.GameID) > 128 {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid reaction")
		}
	}
	for _, report := range snapshot.Reports {
		if strings.TrimSpace(report.ID) == "" || strings.TrimSpace(report.GameID) == "" || strings.TrimSpace(report.ReporterID) == "" ||
			len(report.ID) > 128 || len(report.GameID) > 128 || len(report.ReporterID) > 128 ||
			strings.TrimSpace(report.Reason) == "" || len(report.Reason) > 500 || len(report.ResolvedBy) > 128 || len(report.ResolutionReason) > 500 ||
			(report.Status != "open" && report.Status != "resolved") {
			return postgresBackupSnapshot{}, fmt.Errorf("social graph backup contains an invalid moderation report")
		}
	}

	return snapshot, nil
}

func writePostgresBackupAtomically(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("backup path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
