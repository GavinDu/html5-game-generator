package attestation

// A private PostgreSQL ledger for server-issued replay attempts. Neither
// HTTP handlers nor browser-side score fields may mint an identity or decision.
// This adapter is intentionally not wired to any public verified-score route.
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ServerIssuedAttempt binds the canonical replay identity to server-issued
// authorization scope and issuance time. Both are immutable across every CAS.
type ServerIssuedAttempt struct {
	Identity        Identity
	AuthScopeSHA256 string
	IssuedAt        time.Time
}

func validServerIssuedAttempt(a ServerIssuedAttempt) bool {
	if _, err := NewPending(a.Identity); err != nil {
		return false
	}
	// A PostgreSQL timestamp retains microseconds, not nanoseconds. Require
	// canonical precision so a round-trip cannot silently alter identity.
	_, offset := a.IssuedAt.Zone()
	return validSHA256(a.AuthScopeSHA256) &&
		!a.IssuedAt.IsZero() && offset == 0 &&
		a.IssuedAt.Equal(a.IssuedAt.Truncate(time.Microsecond))
}

// PostgresVerdictStore coordinates distinct process workers using a single
// transactional row lock and a database-global UNIQUE replay job ID.
// No local mutex, journal or process-global state is an authority here.
type PostgresVerdictStore struct {
	db *sql.DB
}

func NewPostgresVerdictStore(db *sql.DB) (*PostgresVerdictStore, error) {
	if db == nil {
		return nil, ErrJournalUnavailable
	}
	return &PostgresVerdictStore{db: db}, nil
}

// Migrate adds only a new server-owned table; it never upgrades historical
// RunRecord.Verified or client-supplied scores. Existing rows are preserved.
func (s *PostgresVerdictStore) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return ErrJournalUnavailable
	}
	const schema = `
CREATE TABLE IF NOT EXISTS runtime_attestation_verdict_v1 (
	run_id TEXT PRIMARY KEY CHECK (octet_length(run_id) BETWEEN 1 AND 128),
	game_id TEXT NOT NULL CHECK (octet_length(game_id) BETWEEN 1 AND 128),
	player_id TEXT NOT NULL CHECK (octet_length(player_id) BETWEEN 1 AND 128),
	auth_scope_sha256 TEXT NOT NULL CHECK (auth_scope_sha256 ~ '^[a-f0-9]{64}$'),
	issued_at TIMESTAMPTZ NOT NULL,
	ir_sha256 TEXT NOT NULL CHECK (ir_sha256 ~ '^[a-f0-9]{64}$'),
	trace_sha256 TEXT NOT NULL CHECK (trace_sha256 ~ '^[a-f0-9]{64}$'),
	capability_sha256 TEXT NOT NULL CHECK (capability_sha256 ~ '^[a-f0-9]{64}$'),
	seed BIGINT NOT NULL CHECK (seed BETWEEN 0 AND 4294967295),
	proof_version TEXT NOT NULL CHECK (proof_version = 'server-replay-verdict-v1'),
	status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','rejected')),
	final_job_id TEXT UNIQUE CHECK (final_job_id ~ '^[a-zA-Z0-9_.-]{1,128}$'),
	score BIGINT NOT NULL DEFAULT 0 CHECK (score BETWEEN -1000000000 AND 1000000000),
	won BOOLEAN NOT NULL DEFAULT FALSE,
	state_sha256 TEXT NOT NULL DEFAULT '',
	rejection_reason TEXT NOT NULL DEFAULT '',
	CONSTRAINT runtime_verdict_shape_v1 CHECK (
		(status = 'pending' AND final_job_id IS NULL AND score = 0 AND NOT won
		 AND state_sha256 = '' AND rejection_reason = '') OR
		(status = 'verified' AND final_job_id IS NOT NULL
		 AND state_sha256 ~ '^[a-f0-9]{64}$' AND rejection_reason = '') OR
		(status = 'rejected' AND final_job_id IS NOT NULL AND score = 0 AND NOT won
		 AND state_sha256 = '' AND rejection_reason IN
		 ('unsupported_ir','invalid_trace','state_mismatch','revoked_attempt',
		  'expired_attempt','replay_internal_error'))
	)
);
CREATE OR REPLACE FUNCTION runtime_attestation_immutable_v1()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF ROW(NEW.run_id, NEW.game_id, NEW.player_id, NEW.auth_scope_sha256,
		NEW.issued_at, NEW.ir_sha256, NEW.trace_sha256,
		NEW.capability_sha256, NEW.seed, NEW.proof_version)
		IS DISTINCT FROM
		ROW(OLD.run_id, OLD.game_id, OLD.player_id, OLD.auth_scope_sha256,
		OLD.issued_at, OLD.ir_sha256, OLD.trace_sha256,
		OLD.capability_sha256, OLD.seed, OLD.proof_version) THEN
		RAISE EXCEPTION 'immutable replay attempt identity' USING ERRCODE = '23514';
	END IF;
	IF OLD.status <> 'pending' AND
		ROW(NEW.status, NEW.final_job_id, NEW.score, NEW.won,
			NEW.state_sha256, NEW.rejection_reason)
		IS DISTINCT FROM
		ROW(OLD.status, OLD.final_job_id, OLD.score, OLD.won,
			OLD.state_sha256, OLD.rejection_reason) THEN
		RAISE EXCEPTION 'immutable terminal replay verdict' USING ERRCODE = '23514';
	END IF;
	RETURN NEW;
END;
$$;
DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_trigger
		WHERE tgname = 'runtime_attestation_immutable_trigger_v1'
		AND tgrelid = 'runtime_attestation_verdict_v1'::regclass) THEN
		CREATE TRIGGER runtime_attestation_immutable_trigger_v1
		BEFORE UPDATE ON runtime_attestation_verdict_v1 FOR EACH ROW
		EXECUTE FUNCTION runtime_attestation_immutable_v1();
	END IF;
END;
$$;
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("%w: migrate server replay verdict storage: %v", ErrJournalUnavailable, err)
	}
	return nil
}

type postgresVerdictEntry struct {
	attempt ServerIssuedAttempt
	verdict Verdict
	jobID   sql.NullString
}

func loadPostgresVerdict(ctx context.Context, tx *sql.Tx, runID string, forUpdate bool) (postgresVerdictEntry, error) {
	query := `SELECT run_id, game_id, player_id, ir_sha256, trace_sha256,
	capability_sha256, seed, proof_version, auth_scope_sha256, issued_at,
	status, final_job_id, score, won, state_sha256, rejection_reason
	FROM runtime_attestation_verdict_v1 WHERE run_id = $1`
	if forUpdate {
		query += " FOR UPDATE"
	}
	var entry postgresVerdictEntry
	var seed int64
	var status string
	err := tx.QueryRowContext(ctx, query, runID).Scan(
		&entry.attempt.Identity.RunID, &entry.attempt.Identity.GameID,
		&entry.attempt.Identity.PlayerID, &entry.attempt.Identity.IRSHA256,
		&entry.attempt.Identity.TraceSHA256, &entry.attempt.Identity.CapabilitySHA256,
		&seed, &entry.attempt.Identity.ProofVersion, &entry.attempt.AuthScopeSHA256,
		&entry.attempt.IssuedAt, &status, &entry.jobID,
		&entry.verdict.Score, &entry.verdict.Won,
		&entry.verdict.StateSHA256, &entry.verdict.RejectionReason,
	)
	if err != nil {
		return postgresVerdictEntry{}, err
	}
	if seed < 0 || seed > 4294967295 {
		return postgresVerdictEntry{}, ErrJournalUnavailable
	}
	entry.attempt.Identity.Seed = uint32(seed)
	entry.verdict.Identity = entry.attempt.Identity
	entry.verdict.Status = Status(status)
	if !validServerIssuedAttempt(entry.attempt) {
		return postgresVerdictEntry{}, ErrJournalUnavailable
	}
	pending, _ := NewPending(entry.attempt.Identity)
	switch entry.verdict.Status {
	case Pending:
		if entry.jobID.Valid || entry.verdict != pending {
			return postgresVerdictEntry{}, ErrJournalUnavailable
		}
	case Verified, Rejected:
		if !entry.jobID.Valid || !validJournalJobID(entry.jobID.String) {
			return postgresVerdictEntry{}, ErrJournalUnavailable
		}
		final, applyErr := ApplyServerDecision(pending, Decision{
			Identity: entry.verdict.Identity, Status: entry.verdict.Status,
			Score: entry.verdict.Score, Won: entry.verdict.Won,
			StateSHA256: entry.verdict.StateSHA256,
			RejectionReason: entry.verdict.RejectionReason,
		})
		if applyErr != nil || final != entry.verdict {
			return postgresVerdictEntry{}, ErrJournalUnavailable
		}
	default:
		return postgresVerdictEntry{}, ErrJournalUnavailable
	}
	return entry, nil
}

func sameIssuedAttempt(a, b ServerIssuedAttempt) bool {
	return a.Identity == b.Identity &&
		a.AuthScopeSHA256 == b.AuthScopeSHA256 &&
		a.IssuedAt.Equal(b.IssuedAt)
}

func postgresVerdictError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23514") {
		return fmt.Errorf("%w: database uniqueness or verdict constraint", ErrJournalConflict)
	}
	return fmt.Errorf("%w: postgres replay verdict operation: %v", ErrJournalUnavailable, err)
}

// CreatePending inserts a canonical server-issued attempt once. Repeating
// the exact attempt is idempotent even after it has reached a terminal state.
func (s *PostgresVerdictStore) CreatePending(ctx context.Context, attempt ServerIssuedAttempt) (Verdict, error) {
	if s == nil || s.db == nil {
		return Verdict{}, ErrJournalUnavailable
	}
	if !validServerIssuedAttempt(attempt) {
		return Verdict{}, ErrInvalidIdentity
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
	INSERT INTO runtime_attestation_verdict_v1
	(run_id, game_id, player_id, auth_scope_sha256, issued_at,
	 ir_sha256, trace_sha256, capability_sha256, seed, proof_version)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	ON CONFLICT (run_id) DO NOTHING`,
		attempt.Identity.RunID, attempt.Identity.GameID, attempt.Identity.PlayerID,
		attempt.AuthScopeSHA256, attempt.IssuedAt, attempt.Identity.IRSHA256,
		attempt.Identity.TraceSHA256, attempt.Identity.CapabilitySHA256,
		int64(attempt.Identity.Seed), attempt.Identity.ProofVersion)
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	entry, err := loadPostgresVerdict(ctx, tx, attempt.Identity.RunID, true)
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	if !sameIssuedAttempt(entry.attempt, attempt) {
		return Verdict{}, ErrJournalConflict
	}
	if err := tx.Commit(); err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	return entry.verdict, nil
}

// CommitServerDecision serializes concurrent workers by locking the current
// run row, then using an optimistic pending-only update. UNIQUE(final_job_id)
// also prevents one replay job from finalizing two different runs. The caller
// must be an authorized independent server reducer, never an HTTP request.
func (s *PostgresVerdictStore) CommitServerDecision(
	ctx context.Context, attempt ServerIssuedAttempt, jobID string, decision Decision,
) (Verdict, error) {
	if s == nil || s.db == nil {
		return Verdict{}, ErrJournalUnavailable
	}
	if !validServerIssuedAttempt(attempt) || decision.Identity != attempt.Identity {
		return Verdict{}, ErrInvalidIdentity
	}
	if !validJournalJobID(jobID) {
		return Verdict{}, ErrInvalidDecision
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	defer tx.Rollback()
	entry, err := loadPostgresVerdict(ctx, tx, attempt.Identity.RunID, true)
	if errors.Is(err, sql.ErrNoRows) {
		return Verdict{}, ErrJournalConflict
	}
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	if !sameIssuedAttempt(entry.attempt, attempt) {
		return Verdict{}, ErrJournalConflict
	}
	result, err := ApplyServerDecision(entry.verdict, decision)
	if err != nil {
		return Verdict{}, err
	}
	if entry.verdict.Status != Pending {
		if !entry.jobID.Valid || entry.jobID.String != jobID {
			return Verdict{}, ErrJournalConflict
		}
		if err := tx.Commit(); err != nil {
			return Verdict{}, postgresVerdictError(err)
		}
		return result, nil
	}
	update, err := tx.ExecContext(ctx, `
	UPDATE runtime_attestation_verdict_v1 SET
		status = $2, final_job_id = $3, score = $4, won = $5,
		state_sha256 = $6, rejection_reason = $7
	WHERE run_id = $1 AND status = 'pending'`,
		attempt.Identity.RunID, string(result.Status), jobID,
		result.Score, result.Won, result.StateSHA256, result.RejectionReason)
	if err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	n, err := update.RowsAffected()
	if err != nil || n != 1 {
		return Verdict{}, ErrJournalConflict
	}
	if err := tx.Commit(); err != nil {
		return Verdict{}, postgresVerdictError(err)
	}
	return result, nil
}
