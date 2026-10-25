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
var ErrCommentIdempotencyConflict = errors.New("idempotency key was already used for a different comment")

type SaveRelation struct {
	ViewerID string
	GameID   string
}

type Comment struct {
	ID        string
	GameID    string
	UserID    string
	Body      string
	CreatedAt time.Time
}

type CommentIdempotencyRelation struct {
	UserID    string
	Key       string
	GameID    string
	Body      string
	CommentID string
	CreatedAt time.Time
}

// Store defines durable social relations shared by all API replicas.
type Store interface {
	Follow(ctx context.Context, viewerID, creatorID string) error
	Unfollow(ctx context.Context, viewerID, creatorID string) error
	ListFollowing(ctx context.Context, viewerID string) ([]string, error)
	ListFollowers(ctx context.Context, creatorID string) ([]string, error)
	Block(ctx context.Context, viewerID, blockedID string) error
	Unblock(ctx context.Context, viewerID, blockedID string) error
	ListBlocked(ctx context.Context, viewerID string) ([]string, error)
	SaveGame(ctx context.Context, viewerID, gameID string) error
	UnsaveGame(ctx context.Context, viewerID, gameID string) error
	ListSavedGames(ctx context.Context, viewerID string) ([]string, error)
	MarkNotInterested(ctx context.Context, viewerID, gameID string) error
	ListNotInterested(ctx context.Context, viewerID string) ([]string, error)
	SetMutedTopics(ctx context.Context, viewerID string, topics []string) error
	ListMutedTopics(ctx context.Context, viewerID string) ([]string, error)
	ResetPreferences(ctx context.Context, viewerID string) error
	CreateComment(ctx context.Context, comment Comment, idempotencyKey string) (Comment, bool, error)
	ListComments(ctx context.Context, gameID string) ([]Comment, error)
	DeleteComment(ctx context.Context, gameID, commentID, actorID string) (found bool, owned bool, err error)
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
CREATE TABLE IF NOT EXISTS social_saves (
  viewer_id TEXT NOT NULL CHECK (length(viewer_id) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (viewer_id, game_id)
);
CREATE INDEX IF NOT EXISTS social_saves_viewer_idx
  ON social_saves (viewer_id, game_id);
CREATE TABLE IF NOT EXISTS social_comments (
  id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  user_id TEXT NOT NULL CHECK (length(user_id) BETWEEN 1 AND 128),
  body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS social_comments_game_created_idx
  ON social_comments (game_id, created_at, id);
CREATE TABLE IF NOT EXISTS social_comment_idempotency (
  user_id TEXT NOT NULL CHECK (length(user_id) BETWEEN 1 AND 128),
  idempotency_key TEXT PRIMARY KEY CHECK (length(idempotency_key) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
  comment_id TEXT NOT NULL CHECK (length(comment_id) BETWEEN 1 AND 128),
  created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS social_not_interested (
  viewer_id TEXT NOT NULL CHECK (length(viewer_id) BETWEEN 1 AND 128),
  game_id TEXT NOT NULL CHECK (length(game_id) BETWEEN 1 AND 128),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (viewer_id, game_id)
);
CREATE INDEX IF NOT EXISTS social_not_interested_game_idx
  ON social_not_interested (game_id, viewer_id);
CREATE TABLE IF NOT EXISTS social_muted_topics (
  viewer_id TEXT NOT NULL CHECK (length(viewer_id) BETWEEN 1 AND 128),
  topic TEXT NOT NULL CHECK (length(topic) BETWEEN 1 AND 40),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (viewer_id, topic)
);
CREATE INDEX IF NOT EXISTS social_muted_topics_topic_idx
  ON social_muted_topics (topic, viewer_id);
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
func (s *PostgresStore) SaveGame(ctx context.Context, viewerID, gameID string) error {
	viewerID, gameID = strings.TrimSpace(viewerID), strings.TrimSpace(gameID)
	if viewerID == "" || gameID == "" || len(viewerID) > 128 || len(gameID) > 128 {
		return errors.New("viewer and game IDs must be 1–128 characters")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO social_saves (viewer_id, game_id) VALUES ($1, $2) ON CONFLICT (viewer_id, game_id) DO NOTHING`, viewerID, gameID)
	if err != nil {
		return fmt.Errorf("persist saved game: %w", err)
	}
	return nil
}

func (s *PostgresStore) UnsaveGame(ctx context.Context, viewerID, gameID string) error {
	viewerID, gameID = strings.TrimSpace(viewerID), strings.TrimSpace(gameID)
	if viewerID == "" || gameID == "" || len(viewerID) > 128 || len(gameID) > 128 {
		return errors.New("viewer and game IDs must be 1–128 characters")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM social_saves WHERE viewer_id = $1 AND game_id = $2`, viewerID, gameID)
	if err != nil {
		return fmt.Errorf("delete saved game: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListSavedGames(ctx context.Context, viewerID string) ([]string, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return nil, errors.New("viewer ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT game_id FROM social_saves WHERE viewer_id = $1 ORDER BY game_id`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("query saved games: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan saved game: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate saved games: %w", err)
	}
	return ids, nil
}

// ImportSaves idempotently imports a local save snapshot in one transaction.
func (s *PostgresStore) ImportSaves(ctx context.Context, relations []SaveRelation) (int, error) {
	for _, relation := range relations {
		viewer, gameID := strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.GameID)
		if viewer == "" || gameID == "" || len(viewer) > 128 || len(gameID) > 128 {
			return 0, fmt.Errorf("invalid saved-game relation in import")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin save import: %w", err)
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO social_saves (viewer_id, game_id) VALUES ($1, $2) ON CONFLICT (viewer_id, game_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("prepare save import: %w", err)
	}
	defer stmt.Close()
	inserted := 0
	for _, relation := range relations {
		result, err := stmt.ExecContext(ctx, strings.TrimSpace(relation.ViewerID), strings.TrimSpace(relation.GameID))
		if err != nil {
			return 0, fmt.Errorf("import saved game: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count imported saved game: %w", err)
		}
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit save import: %w", err)
	}
	return inserted, nil
}
func (s *PostgresStore) CreateComment(ctx context.Context, comment Comment, idempotencyKey string) (Comment, bool, error) {
	comment.ID = strings.TrimSpace(comment.ID)
	comment.GameID = strings.TrimSpace(comment.GameID)
	comment.UserID = strings.TrimSpace(comment.UserID)
	comment.Body = strings.TrimSpace(comment.Body)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if comment.ID == "" || comment.GameID == "" || comment.UserID == "" ||
		len(comment.ID) > 128 || len(comment.GameID) > 128 || len(comment.UserID) > 128 ||
		comment.Body == "" || len(comment.Body) > 2000 || len(idempotencyKey) > 128 {
		return Comment{}, false, errors.New("comment IDs must be 1–128 characters, body 1–2000 characters, and idempotency key at most 128 characters")
	}
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Comment{}, false, fmt.Errorf("begin comment transaction: %w", err)
	}
	defer tx.Rollback()

	if idempotencyKey != "" {
		prior, found, err := loadIdempotentComment(ctx, tx, idempotencyKey)
		if err != nil {
			return Comment{}, false, err
		}
		if found {
			if prior.UserID != comment.UserID || prior.GameID != comment.GameID || prior.Body != comment.Body {
				return Comment{}, false, ErrCommentIdempotencyConflict
			}
			if err := tx.Commit(); err != nil {
				return Comment{}, false, fmt.Errorf("commit idempotent comment read: %w", err)
			}
			return prior, true, nil
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO social_comments (id, game_id, user_id, body, created_at)
VALUES ($1, $2, $3, $4, $5)`, comment.ID, comment.GameID, comment.UserID, comment.Body, comment.CreatedAt); err != nil {
		return Comment{}, false, fmt.Errorf("insert comment: %w", err)
	}
	if idempotencyKey != "" {
		result, err := tx.ExecContext(ctx, `
INSERT INTO social_comment_idempotency
(user_id, idempotency_key, game_id, body, comment_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (idempotency_key) DO NOTHING`,
			comment.UserID, idempotencyKey, comment.GameID, comment.Body, comment.ID, comment.CreatedAt)
		if err != nil {
			return Comment{}, false, fmt.Errorf("persist comment idempotency key: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return Comment{}, false, fmt.Errorf("check comment idempotency insert: %w", err)
		}
		if inserted == 0 {
			// A concurrent request won the same idempotency key. Roll back the
			// speculative comment before loading the winner in a fresh query.
			_ = tx.Rollback()
			prior, found, err := loadIdempotentComment(ctx, s.db, idempotencyKey)
			if err != nil {
				return Comment{}, false, err
			}
			if !found {
				return Comment{}, false, errors.New("idempotency record disappeared after conflict")
			}
			if prior.GameID != comment.GameID || prior.Body != comment.Body {
				return Comment{}, false, ErrCommentIdempotencyConflict
			}
			return prior, true, nil
		}
	}
	if err := tx.Commit(); err != nil {
		return Comment{}, false, fmt.Errorf("commit comment: %w", err)
	}
	return comment, false, nil
}

type commentQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadIdempotentComment(ctx context.Context, q commentQueryer, key string) (Comment, bool, error) {
	var comment Comment
	err := q.QueryRowContext(ctx, `
SELECT comment_id, game_id, user_id, body, created_at
FROM social_comment_idempotency
WHERE idempotency_key = $1`, key).Scan(
		&comment.ID, &comment.GameID, &comment.UserID, &comment.Body, &comment.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Comment{}, false, nil
	}
	if err != nil {
		return Comment{}, false, fmt.Errorf("read comment idempotency record: %w", err)
	}
	return comment, true, nil
}

func (s *PostgresStore) ListComments(ctx context.Context, gameID string) ([]Comment, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" || len(gameID) > 128 {
		return nil, errors.New("game ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, game_id, user_id, body, created_at
FROM social_comments WHERE game_id = $1
ORDER BY created_at, id`, gameID)
	if err != nil {
		return nil, fmt.Errorf("query comments: %w", err)
	}
	defer rows.Close()
	comments := make([]Comment, 0)
	for rows.Next() {
		var comment Comment
		if err := rows.Scan(&comment.ID, &comment.GameID, &comment.UserID, &comment.Body, &comment.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate comments: %w", err)
	}
	return comments, nil
}

func (s *PostgresStore) DeleteComment(ctx context.Context, gameID, commentID, actorID string) (bool, bool, error) {
	gameID, commentID, actorID = strings.TrimSpace(gameID), strings.TrimSpace(commentID), strings.TrimSpace(actorID)
	if gameID == "" || commentID == "" || actorID == "" || len(gameID) > 128 || len(commentID) > 128 || len(actorID) > 128 {
		return false, false, errors.New("game, comment, and actor IDs must be 1–128 characters")
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM social_comments WHERE game_id = $1 AND id = $2 AND user_id = $3`,
		gameID, commentID, actorID)
	if err != nil {
		return false, false, fmt.Errorf("delete comment: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, false, fmt.Errorf("check deleted comment: %w", err)
	}
	if n == 1 {
		return true, true, nil
	}
	var owner string
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM social_comments WHERE game_id = $1 AND id = $2`, gameID, commentID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("find comment after delete miss: %w", err)
	}
	return true, false, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// MarkNotInterested is an idempotent viewer/game preference mutation.
func (s *PostgresStore) MarkNotInterested(ctx context.Context, viewerID, gameID string) error {
	viewerID, gameID = strings.TrimSpace(viewerID), strings.TrimSpace(gameID)
	if viewerID == "" || gameID == "" || len(viewerID) > 128 || len(gameID) > 128 {
		return errors.New("viewer and game IDs must be 1–128 characters")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO social_not_interested (viewer_id, game_id)
VALUES ($1, $2)
ON CONFLICT (viewer_id, game_id) DO NOTHING`, viewerID, gameID)
	if err != nil {
		return fmt.Errorf("persist not-interested preference: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListNotInterested(ctx context.Context, viewerID string) ([]string, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return nil, errors.New("viewer ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT game_id FROM social_not_interested WHERE viewer_id = $1 ORDER BY game_id`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("query not-interested preferences: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan not-interested preference: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate not-interested preferences: %w", err)
	}
	return ids, nil
}

// SetMutedTopics replaces a viewer's complete normalized topic preference set
// atomically. Repeating the same request is safe and leaves the same state.
func (s *PostgresStore) SetMutedTopics(ctx context.Context, viewerID string, topics []string) error {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return errors.New("viewer ID must be 1–128 characters")
	}
	normalized := make([]string, 0, len(topics))
	seen := map[string]bool{}
	for _, raw := range topics {
		topic := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "#")))
		if topic == "" || len(topic) > 40 {
			return errors.New("topics must contain 1–40 characters")
		}
		if !seen[topic] {
			seen[topic] = true
			normalized = append(normalized, topic)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin muted-topic update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM social_muted_topics WHERE viewer_id = $1`, viewerID); err != nil {
		return fmt.Errorf("clear muted topics: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO social_muted_topics (viewer_id, topic)
VALUES ($1, $2)
ON CONFLICT (viewer_id, topic) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare muted-topic update: %w", err)
	}
	defer stmt.Close()
	for _, topic := range normalized {
		if _, err := stmt.ExecContext(ctx, viewerID, topic); err != nil {
			return fmt.Errorf("persist muted topic: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit muted-topic update: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListMutedTopics(ctx context.Context, viewerID string) ([]string, error) {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return nil, errors.New("viewer ID must be 1–128 characters")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT topic FROM social_muted_topics WHERE viewer_id = $1 ORDER BY topic`, viewerID)
	if err != nil {
		return nil, fmt.Errorf("query muted topics: %w", err)
	}
	defer rows.Close()
	topics := make([]string, 0)
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			return nil, fmt.Errorf("scan muted topic: %w", err)
		}
		topics = append(topics, topic)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate muted topics: %w", err)
	}
	return topics, nil
}

func (s *PostgresStore) ResetPreferences(ctx context.Context, viewerID string) error {
	viewerID = strings.TrimSpace(viewerID)
	if viewerID == "" || len(viewerID) > 128 {
		return errors.New("viewer ID must be 1–128 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin preference reset: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM social_muted_topics WHERE viewer_id = $1`, viewerID); err != nil {
		return fmt.Errorf("delete muted topics: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM social_not_interested WHERE viewer_id = $1`, viewerID); err != nil {
		return fmt.Errorf("delete not-interested preferences: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit preference reset: %w", err)
	}
	return nil
}
