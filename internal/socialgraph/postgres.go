package socialgraph

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var ErrSelfFollow = errors.New("a user cannot follow themselves")
var ErrSelfBlock = errors.New("a user cannot block themselves")

// Store defines durable follow and block relations shared by all API replicas.
type Store interface {
	Follow(ctx context.Context, viewerID, creatorID string) error
	Unfollow(ctx context.Context, viewerID, creatorID string) error
	ListFollowing(ctx context.Context, viewerID string) ([]string, error)
	ListFollowers(ctx context.Context, creatorID string) ([]string, error)
	Block(ctx context.Context, viewerID, blockedID string) error
	Unblock(ctx context.Context, viewerID, blockedID string) error
	ListBlocked(ctx context.Context, viewerID string) ([]string, error)
	Close() error
}

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres DSN is empty")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open social graph database: %w", err)
	}
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect social graph database: %w", err)
	}
	s := &PostgresStore{db: db}
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *PostgresStore) Migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS social_follows (
  viewer_id TEXT NOT NULL CHECK (length(viewer_id) BETWEEN 1 AND 128),
  creator_id TEXT NOT NULL CHECK (length(creator_id) BETWEEN 1 AND 128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (viewer_id, creator_id),
  CHECK (viewer_id <> creator_id)
);
CREATE INDEX IF NOT EXISTS social_follows_creator_idx
  ON social_follows (creator_id, created_at DESC);
CREATE TABLE IF NOT EXISTS social_blocks (
  viewer_id TEXT NOT NULL CHECK (length(viewer_id) BETWEEN 1 AND 128),
  blocked_id TEXT NOT NULL CHECK (length(blocked_id) BETWEEN 1 AND 128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (viewer_id, blocked_id),
  CHECK (viewer_id <> blocked_id)
);
CREATE INDEX IF NOT EXISTS social_blocks_blocked_idx
  ON social_blocks (blocked_id, created_at DESC);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate social graph schema: %w", err)
	}
	return nil
}

func (s *PostgresStore) Follow(ctx context.Context, viewerID, creatorID string) error {
	viewerID, creatorID = strings.TrimSpace(viewerID), strings.TrimSpace(creatorID)
	if viewerID == "" || creatorID == "" || len(viewerID) > 128 || len(creatorID) > 128 {
		return errors.New("viewer and creator IDs must be 1–128 characters")
	}
	if viewerID == creatorID {
		return ErrSelfFollow
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO social_follows (viewer_id, creator_id)
VALUES ($1, $2)
ON CONFLICT (viewer_id, creator_id) DO NOTHING`, viewerID, creatorID)
	if err != nil {
		return fmt.Errorf("persist follow edge: %w", err)
	}
	return nil
}

func (s *PostgresStore) Unfollow(ctx context.Context, viewerID, creatorID string) error {
	viewerID, creatorID = strings.TrimSpace(viewerID), strings.TrimSpace(creatorID)
	if viewerID == "" || creatorID == "" || len(viewerID) > 128 || len(creatorID) > 128 {
		return errors.New("viewer and creator IDs must be 1–128 characters")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM social_follows WHERE viewer_id = $1 AND creator_id = $2`,
		viewerID, creatorID)
	if err != nil {
		return fmt.Errorf("delete follow edge: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListFollowing(ctx context.Context, viewerID string) ([]string, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return nil, errors.New("viewer ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT creator_id FROM social_follows WHERE viewer_id = $1 ORDER BY creator_id`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("query following edges: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan following edge: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate following edges: %w", err)
	}
	return ids, nil
}

func (s *PostgresStore) ListFollowers(ctx context.Context, creatorID string) ([]string, error) {
	creatorID = strings.TrimSpace(creatorID)
	if creatorID == "" || len(creatorID) > 128 {
		return nil, errors.New("creator ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT viewer_id FROM social_follows WHERE creator_id = $1 ORDER BY viewer_id`, creatorID)
	if err != nil {
		return nil, fmt.Errorf("query follower edges: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan follower edge: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate follower edges: %w", err)
	}
	return ids, nil
}

type Edge struct {
	ViewerID  string
	CreatorID string
}

type BlockRelation struct {
	ViewerID  string
	BlockedID string
}

// ImportFollows idempotently imports a snapshot's follow edges in one
// transaction. It never deletes destination rows and is safe to resume.
func (s *PostgresStore) ImportFollows(ctx context.Context, edges []Edge) (int, error) {
	for _, edge := range edges {
		viewer, creator := strings.TrimSpace(edge.ViewerID), strings.TrimSpace(edge.CreatorID)
		if viewer == "" || creator == "" || len(viewer) > 128 || len(creator) > 128 || viewer == creator {
			return 0, fmt.Errorf("invalid follow edge in import")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin follow import: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO social_follows (viewer_id, creator_id)
VALUES ($1, $2)
ON CONFLICT (viewer_id, creator_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("prepare follow import: %w", err)
	}
	defer stmt.Close()
	inserted := 0
	for _, edge := range edges {
		viewer, creator := strings.TrimSpace(edge.ViewerID), strings.TrimSpace(edge.CreatorID)
		if viewer == "" || creator == "" || len(viewer) > 128 || len(creator) > 128 || viewer == creator {
			return 0, fmt.Errorf("invalid follow edge in import")
		}
		result, err := stmt.ExecContext(ctx, viewer, creator)
		if err != nil {
			return 0, fmt.Errorf("import follow edge: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count imported follow edge: %w", err)
		}
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit follow import: %w", err)
	}
	return inserted, nil
}

func (s *PostgresStore) Block(ctx context.Context, viewerID, blockedID string) error {
	viewerID, blockedID = strings.TrimSpace(viewerID), strings.TrimSpace(blockedID)
	if viewerID == "" || blockedID == "" || len(viewerID) > 128 || len(blockedID) > 128 {
		return errors.New("viewer and blocked IDs must be 1–128 characters")
	}
	if viewerID == blockedID {
		return ErrSelfBlock
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO social_blocks (viewer_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT (viewer_id, blocked_id) DO NOTHING`, viewerID, blockedID)
	if err != nil {
		return fmt.Errorf("persist block relation: %w", err)
	}
	return nil
}

func (s *PostgresStore) Unblock(ctx context.Context, viewerID, blockedID string) error {
	viewerID, blockedID = strings.TrimSpace(viewerID), strings.TrimSpace(blockedID)
	if viewerID == "" || blockedID == "" || len(viewerID) > 128 || len(blockedID) > 128 {
		return errors.New("viewer and blocked IDs must be 1–128 characters")
	}
	if viewerID == blockedID {
		return ErrSelfBlock
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM social_blocks WHERE viewer_id = $1 AND blocked_id = $2`, viewerID, blockedID)
	if err != nil {
		return fmt.Errorf("delete block relation: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListBlocked(ctx context.Context, viewerID string) ([]string, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return nil, errors.New("viewer ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT blocked_id FROM social_blocks WHERE viewer_id = $1 ORDER BY blocked_id`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("query block relations: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan block relation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate block relations: %w", err)
	}
	return ids, nil
}

// ImportBlocks idempotently imports a block snapshot in one transaction.
// It never deletes destination rows and can safely resume after interruption.
func (s *PostgresStore) ImportBlocks(ctx context.Context, relations []BlockRelation) (int, error) {
	for _, relation := range relations {
		viewer, blocked := strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.BlockedID)
		if viewer == "" || blocked == "" || len(viewer) > 128 || len(blocked) > 128 || viewer == blocked {
			return 0, fmt.Errorf("invalid block relation in import")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin block import: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO social_blocks (viewer_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT (viewer_id, blocked_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("prepare block import: %w", err)
	}
	defer stmt.Close()
	inserted := 0
	for _, relation := range relations {
		result, err := stmt.ExecContext(ctx, strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.BlockedID))
		if err != nil {
			return 0, fmt.Errorf("import block relation: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count imported block relation: %w", err)
		}
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit block import: %w", err)
	}
	return inserted, nil
}
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
