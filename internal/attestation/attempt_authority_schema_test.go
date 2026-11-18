package attestation

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type privateAuthorityFixture struct {
	runID, nonceSHA256 string
	seed               uint32
	issued, expires    time.Time
}

func newPrivateAuthorityFixture(t *testing.T) privateAuthorityFixture {
	t.Helper()
	entropy, err := newProductionAttemptEntropy()
	if err != nil {
		t.Fatal(err)
	}
	issued := time.Now().UTC().Truncate(time.Microsecond)
	return privateAuthorityFixture{
		runID: entropy.RunID, nonceSHA256: entropy.NonceSHA256,
		seed: entropy.Seed, issued: issued, expires: issued.Add(2 * time.Minute),
	}
}

func insertPrivateAuthorityFixture(ctx context.Context, db *sql.DB, f privateAuthorityFixture) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO runtime_attestation_attempt_authority_v1
		(run_id, nonce_sha256, game_id, player_id, revision_id,
		 ir_sha256, capability_sha256, auth_scope_sha256, credential_epoch_sha256,
		 seed, proof_version, issued_at, expires_at)
		VALUES ($1,$2,'published_game','authenticated_subject','revision_1',
		 $3,$4,$5,$6,$7,$8,$9,$10)`,
		f.runID, f.nonceSHA256,
		strings.Repeat("a", 64), strings.Repeat("b", 64),
		strings.Repeat("c", 64), strings.Repeat("d", 64),
		int64(f.seed), ProofVersion, f.issued, f.expires)
	return err
}

func cleanupPrivateAuthorityFixture(t *testing.T, db *sql.DB, f privateAuthorityFixture) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx, `
			DELETE FROM runtime_attestation_attempt_authority_v1
			WHERE run_id=$1`, f.runID); err != nil {
			t.Errorf("clean up isolated private-authority fixture: %v", err)
		}
	})
}

func TestPrivateAuthoritySchemaRejectsInvalidOrReusedIdentityAcrossConnections(t *testing.T) {
	a, b, db, ctx := postgresAttestationTestStores(t)
	if err := migratePrivateAttemptAuthoritySchema(ctx, a.db); err != nil {
		t.Fatal(err)
	}
	if err := migratePrivateAttemptAuthoritySchema(ctx, b.db); err != nil {
		t.Fatalf("repeated migration should preserve authority table: %v", err)
	}
	first := newPrivateAuthorityFixture(t)
	cleanupPrivateAuthorityFixture(t, db, first)
	if err := insertPrivateAuthorityFixture(ctx, a.db, first); err != nil {
		t.Fatal(err)
	}
	// Independent connection/process can read the durable immutable grant.
	var nonce, proof string
	if err := b.db.QueryRowContext(ctx, `
		SELECT nonce_sha256, proof_version
		FROM runtime_attestation_attempt_authority_v1 WHERE run_id=$1`,
		first.runID).Scan(&nonce, &proof); err != nil {
		t.Fatal(err)
	}
	if nonce != first.nonceSHA256 || proof != ProofVersion {
		t.Fatalf("durable grant changed across DB connections: %q %q", nonce, proof)
	}
	// This schema is storage only: inserting a row did not create a pending
	// or verified verdict, even though the old CAS table already exists.
	var verdictRows int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runtime_attestation_verdict_v1
		WHERE run_id=$1`, first.runID).Scan(&verdictRows); err != nil || verdictRows != 0 {
		t.Fatalf("private schema minted a verdict: rows=%d err=%v", verdictRows, err)
	}
	duplicateID := newPrivateAuthorityFixture(t)
	duplicateID.runID = first.runID
	if err := insertPrivateAuthorityFixture(ctx, b.db, duplicateID); err == nil {
		t.Fatal("same CSPRNG run ID with different nonce must conflict")
	}
	duplicateNonce := newPrivateAuthorityFixture(t)
	duplicateNonce.nonceSHA256 = first.nonceSHA256
	if err := insertPrivateAuthorityFixture(ctx, b.db, duplicateNonce); err == nil {
		t.Fatal("same nonce binding with different ID must conflict")
	}
	// Race a fresh shared nonce between two independent PostgreSQL
	// connections. One and only one INSERT may own its immutable nonce;
	// this cannot be protected by a per-process mutex.
	raceA := newPrivateAuthorityFixture(t)
	raceB := newPrivateAuthorityFixture(t)
	raceB.nonceSHA256 = raceA.nonceSHA256
	cleanupPrivateAuthorityFixture(t, db, raceA)
	cleanupPrivateAuthorityFixture(t, db, raceB)
	var insertWG sync.WaitGroup
	inserts := make(chan error, 2)
	for _, pair := range []struct {
		conn *sql.DB
		row  privateAuthorityFixture
	}{{a.db, raceA}, {b.db, raceB}} {
		insertWG.Add(1)
		go func(conn *sql.DB, row privateAuthorityFixture) {
			defer insertWG.Done()
			inserts <- insertPrivateAuthorityFixture(ctx, conn, row)
		}(pair.conn, pair.row)
	}
	insertWG.Wait()
	close(inserts)
	var inserted, duplicate int
	for err := range inserts {
		if err == nil {
			inserted++
		} else {
			duplicate++
		}
	}
	if inserted != 1 || duplicate != 1 {
		t.Fatalf("nonce collision across connections: inserted=%d denied=%d", inserted, duplicate)
	}
	shortTTL := newPrivateAuthorityFixture(t)
	shortTTL.expires = shortTTL.issued
	if err := insertPrivateAuthorityFixture(ctx, a.db, shortTTL); err == nil {
		t.Fatal("zero-duration attempt must be rejected")
	}
	longTTL := newPrivateAuthorityFixture(t)
	longTTL.expires = longTTL.issued.Add(5*time.Minute + time.Microsecond)
	if err := insertPrivateAuthorityFixture(ctx, a.db, longTTL); err == nil {
		t.Fatal("TTL over 5m must be rejected by the DB")
	}
	invalidSeed := newPrivateAuthorityFixture(t)
	// The Go candidate is uint32; a direct malicious SQL client must still
	// be unable to write a BIGINT seed outside uint32.
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO runtime_attestation_attempt_authority_v1
		(run_id,nonce_sha256,game_id,player_id,revision_id,ir_sha256,
		 capability_sha256,auth_scope_sha256,credential_epoch_sha256,
		 seed,proof_version,issued_at,expires_at)
		VALUES ($1,$2,'g','p','v',$3,$3,$3,$3,4294967296,$4,$5,$6)`,
		invalidSeed.runID, invalidSeed.nonceSHA256, strings.Repeat("a", 64),
		ProofVersion, invalidSeed.issued, invalidSeed.expires); err == nil {
		t.Fatal("out-of-range direct-SQL seed accepted")
	}
	// SQL cannot rewrite immutable scope, expiry or seed after issuance.
	for _, update := range []string{
		"UPDATE runtime_attestation_attempt_authority_v1 SET seed=seed+1 WHERE run_id=$1",
		"UPDATE runtime_attestation_attempt_authority_v1 SET expires_at=expires_at+interval '1 second' WHERE run_id=$1",
		"UPDATE runtime_attestation_attempt_authority_v1 SET credential_epoch_sha256=$2 WHERE run_id=$1",
	} {
		var err error
		if strings.Contains(update, "$2") {
			_, err = b.db.ExecContext(ctx, update, first.runID, strings.Repeat("f", 64))
		} else {
			_, err = b.db.ExecContext(ctx, update, first.runID)
		}
		if err == nil {
			t.Fatalf("SQL rewrote immutable issued identity: %s", update)
		}
	}
}

func TestPrivateAuthorityRevocationIsOneWayAcrossConcurrentDBConnections(t *testing.T) {
	a, b, db, ctx := postgresAttestationTestStores(t)
	if err := migratePrivateAttemptAuthoritySchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	f := newPrivateAuthorityFixture(t)
	cleanupPrivateAuthorityFixture(t, db, f)
	if err := insertPrivateAuthorityFixture(ctx, a.db, f); err != nil {
		t.Fatal(err)
	}
	// Two real DB clients race to revoke; exactly one may write the one-way
	// transition. No caller can convert a revoked attempt back to issued.
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	errs := make(chan error, 2)
	revokedAt := f.issued.Add(time.Second)
	for _, dbConn := range []*sql.DB{a.db, b.db} {
		wg.Add(1)
		go func(conn *sql.DB) {
			defer wg.Done()
			result, err := conn.ExecContext(ctx, `
				UPDATE runtime_attestation_attempt_authority_v1
				SET revoked_at=$2, revocation_reason='credential_rotated'
				WHERE run_id=$1 AND revoked_at IS NULL`, f.runID, revokedAt)
			if err != nil {
				errs <- err
				return
			}
			n, err := result.RowsAffected()
			if err != nil {
				errs <- err
				return
			}
			results <- n
		}(dbConn)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var successes, noops int
	for n := range results {
		switch n {
		case 1:
			successes++
		case 0:
			noops++
		default:
			t.Fatalf("unexpected number of revoked rows: %d", n)
		}
	}
	if successes != 1 || noops != 1 {
		t.Fatalf("expected one atomic revocation: success=%d noop=%d", successes, noops)
	}
	var persistedAt time.Time
	var reason string
	if err := b.db.QueryRowContext(ctx, `
		SELECT revoked_at, revocation_reason
		FROM runtime_attestation_attempt_authority_v1 WHERE run_id=$1`,
		f.runID).Scan(&persistedAt, &reason); err != nil {
		t.Fatal(err)
	}
	if !persistedAt.Equal(revokedAt) || reason != "credential_rotated" {
		t.Fatalf("restarted DB connection lost revocation: %v %q", persistedAt, reason)
	}
	for label, mutation := range map[string]string{
		"unrevoke": "SET revoked_at=NULL, revocation_reason=''",
		"rewrite": "SET revoked_at=revoked_at+interval '1 second'",
		"reason":  "SET revocation_reason='manual_revoke'",
		"scope":   "SET auth_scope_sha256='" + strings.Repeat("f", 64) + "'",
	} {
		query := fmt.Sprintf("UPDATE runtime_attestation_attempt_authority_v1 %s WHERE run_id=$1", mutation)
		if _, err := a.db.ExecContext(ctx, query, f.runID); err == nil {
			t.Fatalf("%s rewrote revoked grant", label)
		}
	}
	// Storage DDL has no grant/worker privileges. This migration does NOT
	// create even a pending verdict, even after grant revocation. A privileged
	// client could still use the preexisting verdict table directly: #462
	// must enforce restricted DB roles and transactional authorization.
	var count int
	if err := b.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runtime_attestation_verdict_v1
		WHERE run_id=$1`, f.runID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("private authority schema created a verdict: count=%d error=%v", count, err)
	}
}
