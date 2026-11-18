package attestation

// Private schema ONLY. This table does not issue replay grants or authorize
// scores: the authenticated issuer (#461), transactional worker fence (#462)
// and independent Go execution (#382) are NOT wired to it.
import (
	"context"
	"database/sql"
	"fmt"
)

// migratePrivateAttemptAuthoritySchema is deliberately package-private. It
// adds no HTTP handler and does not modify the existing pending/verdict table.
// Actual database-role privileges must be enforced by the eventual issuer,
// not inferred merely from a private Go function or a SQL CHECK constraint.
func migratePrivateAttemptAuthoritySchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return ErrJournalUnavailable
	}
	const schema = `
CREATE TABLE IF NOT EXISTS runtime_attestation_attempt_authority_v1 (
	run_id TEXT PRIMARY KEY CHECK (run_id ~ '^att_[a-f0-9]{64}$'),
	nonce_sha256 TEXT NOT NULL UNIQUE CHECK (nonce_sha256 ~ '^[a-f0-9]{64}$'),
	game_id TEXT NOT NULL CHECK (game_id ~ '^[a-zA-Z0-9_-]{1,128}$'),
	player_id TEXT NOT NULL CHECK (octet_length(player_id) BETWEEN 1 AND 128),
	revision_id TEXT NOT NULL CHECK (revision_id ~ '^[a-zA-Z0-9_-]{1,128}$'),
	ir_sha256 TEXT NOT NULL CHECK (ir_sha256 ~ '^[a-f0-9]{64}$'),
	capability_sha256 TEXT NOT NULL CHECK (capability_sha256 ~ '^[a-f0-9]{64}$'),
	auth_scope_sha256 TEXT NOT NULL CHECK (auth_scope_sha256 ~ '^[a-f0-9]{64}$'),
	credential_epoch_sha256 TEXT NOT NULL CHECK (credential_epoch_sha256 ~ '^[a-f0-9]{64}$'),
	seed BIGINT NOT NULL CHECK (seed BETWEEN 0 AND 4294967295),
	proof_version TEXT NOT NULL CHECK (proof_version = 'server-replay-verdict-v1'),
	issued_at TIMESTAMPTZ NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ,
	revocation_reason TEXT NOT NULL DEFAULT '',
	CONSTRAINT runtime_attempt_expiry_v1 CHECK (
		expires_at > issued_at AND
		expires_at <= issued_at + interval '5 minutes'
	),
	CONSTRAINT runtime_attempt_revocation_v1 CHECK (
		(revoked_at IS NULL AND revocation_reason = '') OR
		(revoked_at IS NOT NULL AND revoked_at >= issued_at AND
		 revocation_reason IN ('logout', 'credential_rotated',
		 'publication_removed', 'moderation_takedown', 'expired',
		 'manual_revoke'))
	)
);
CREATE OR REPLACE FUNCTION runtime_attempt_authority_immutable_v1()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF ROW(NEW.run_id, NEW.nonce_sha256, NEW.game_id, NEW.player_id,
		NEW.revision_id, NEW.ir_sha256, NEW.capability_sha256,
		NEW.auth_scope_sha256, NEW.credential_epoch_sha256,
		NEW.seed, NEW.proof_version, NEW.issued_at, NEW.expires_at)
		IS DISTINCT FROM
		ROW(OLD.run_id, OLD.nonce_sha256, OLD.game_id, OLD.player_id,
		OLD.revision_id, OLD.ir_sha256, OLD.capability_sha256,
		OLD.auth_scope_sha256, OLD.credential_epoch_sha256,
		OLD.seed, OLD.proof_version, OLD.issued_at, OLD.expires_at) THEN
		RAISE EXCEPTION 'immutable replay attempt authorization'
			USING ERRCODE = '23514';
	END IF;
	IF OLD.revoked_at IS NOT NULL AND
		ROW(NEW.revoked_at, NEW.revocation_reason)
		IS DISTINCT FROM ROW(OLD.revoked_at, OLD.revocation_reason) THEN
		RAISE EXCEPTION 'immutable revoked replay attempt'
			USING ERRCODE = '23514';
	END IF;
	RETURN NEW;
END;
$$;
DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_trigger
		WHERE tgname = 'runtime_attempt_authority_immutable_trigger_v1'
		AND tgrelid = 'runtime_attestation_attempt_authority_v1'::regclass) THEN
		CREATE TRIGGER runtime_attempt_authority_immutable_trigger_v1
		BEFORE UPDATE ON runtime_attestation_attempt_authority_v1 FOR EACH ROW
		EXECUTE FUNCTION runtime_attempt_authority_immutable_v1();
	END IF;
END;
$$;
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("%w: migrate private attempt authority schema: %v", ErrJournalUnavailable, err)
	}
	return nil
}
