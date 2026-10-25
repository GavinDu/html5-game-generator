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
)

const postgresBackupFormatVersion = 1

type postgresBackupEnvelope struct {
	FormatVersion int             `json:"formatVersion"`
	SHA256        string          `json:"sha256"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

type postgresBackupSnapshot struct {
	Follows []Edge          `json:"follows"`
	Blocks  []BlockRelation `json:"blocks"`
}

// BackupTo exports a consistent, checksummed snapshot of shared follow and
// block relations. The backup is written with restrictive permissions and an
// atomic rename so incomplete files are not mistaken for valid backups.
func (s *PostgresStore) BackupTo(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin social graph backup: %w", err)
	}
	defer tx.Rollback()
	snapshot := postgresBackupSnapshot{
		Follows: make([]Edge, 0),
		Blocks:  make([]BlockRelation, 0),
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
	if envelope.FormatVersion != postgresBackupFormatVersion {
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
	if snapshot.Follows == nil || snapshot.Blocks == nil {
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
