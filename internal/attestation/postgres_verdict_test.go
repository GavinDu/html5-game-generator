package attestation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func postgresAttestationAttempt(name string) ServerIssuedAttempt {
	id := policyIdentity()
	id.RunID = fmt.Sprintf("%s-%d", name, time.Now().UTC().UnixNano())
	return ServerIssuedAttempt{
		Identity: id, AuthScopeSHA256: strings.Repeat("d", 64),
		IssuedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func postgresAttestationTestStores(t *testing.T) (*PostgresVerdictStore, *PostgresVerdictStore, *sql.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("DQ_SOCIAL_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration requires DQ_SOCIAL_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	first, err := sql.Open("postgres", dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	second, err := sql.Open("postgres", dsn)
	if err != nil {
		first.Close()
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		first.Close()
		second.Close()
	})
	a, err := NewPostgresVerdictStore(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewPostgresVerdictStore(second)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return a, b, first, ctx
}

func removePostgresAttestationTestRow(t *testing.T, db *sql.DB, runID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if _, err := db.ExecContext(ctx,
			"DELETE FROM runtime_attestation_verdict_v1 WHERE run_id = $1", runID); err != nil {
			t.Errorf("clean up isolated verdict fixture: %v", err)
		}
	})
}

func TestPostgresVerdictPrivateIdentityRejectsMalformedIssuance(t *testing.T) {
	a := postgresAttestationAttempt("issue")
	if !validServerIssuedAttempt(a) {
		t.Fatal("canonical server issuance rejected")
	}
	for _, mutate := range []func(*ServerIssuedAttempt){
		func(a *ServerIssuedAttempt) { a.AuthScopeSHA256 = strings.Repeat("A", 64) },
		func(a *ServerIssuedAttempt) { a.AuthScopeSHA256 = strings.Repeat("a", 12) },
		func(a *ServerIssuedAttempt) { a.IssuedAt = time.Time{} },
		func(a *ServerIssuedAttempt) { a.IssuedAt = a.IssuedAt.Add(time.Nanosecond) },
		func(a *ServerIssuedAttempt) { a.Identity.ProofVersion = "client-v1" },
		func(a *ServerIssuedAttempt) { a.Identity.IRSHA256 = strings.Repeat("f", 12) },
	} {
		bad := a
		mutate(&bad)
		if validServerIssuedAttempt(bad) {
			t.Fatalf("untrusted attempted issuance accepted: %+v", bad)
		}
	}
	if _, err := NewPostgresVerdictStore(nil); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("nil DB must fail closed: %v", err)
	}
}

func TestPostgresVerdictCASAcrossTwoIndependentConnections(t *testing.T) {
	a, b, db, ctx := postgresAttestationTestStores(t)
	attempt := postgresAttestationAttempt("cas")
	removePostgresAttestationTestRow(t, db, attempt.Identity.RunID)
	pending, err := a.CreatePending(ctx, attempt)
	if err != nil || pending.Status != Pending {
		t.Fatalf("create pending: %+v %v", pending, err)
	}
	again, err := b.CreatePending(ctx, attempt)
	if err != nil || again != pending {
		t.Fatalf("other connection lost pending: %+v %v", again, err)
	}

	verified := Decision{Identity: attempt.Identity, Status: Verified, Score: 6,
		Won: true, StateSHA256: strings.Repeat("e", 64)}
	rejected := Decision{Identity: attempt.Identity, Status: Rejected,
		RejectionReason: "unsupported_ir"}
	const workers = 18
	type response struct {
		verdict Verdict
		err     error
		jobID   string
	}
	out := make(chan response, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store, decision, job := a, verified, "cas-worker-verifies"
			if i%2 != 0 {
				store, decision, job = b, rejected, "cas-worker-rejects"
			}
			v, err := store.CommitServerDecision(ctx, attempt, job, decision)
			out <- response{verdict: v, err: err, jobID: job}
		}(i)
	}
	wg.Wait()
	close(out)
	var success *response
	conflicts := 0
	for result := range out {
		if result.err != nil {
			if !errors.Is(result.err, ErrJournalConflict) &&
				!errors.Is(result.err, ErrConflictingVerdict) {
				t.Fatalf("unexpected transactional error: %v", result.err)
			}
			conflicts++
			continue
		}
		if success == nil {
			copy := result
			success = &copy
		} else if success.jobID != result.jobID || success.verdict != result.verdict {
			t.Fatalf("independent workers finalized conflicting outcomes: %+v and %+v", success, result)
		}
	}
	if success == nil || conflicts == 0 {
		t.Fatalf("concurrent CAS needed one winner and hard conflicts: success=%+v conflicts=%d", success, conflicts)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	row, err := loadPostgresVerdict(ctx, tx, attempt.Identity.RunID, false)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if row.verdict != success.verdict || row.jobID.String != success.jobID {
		t.Fatalf("database and workers disagree about sole terminal verdict: %+v %+v", row, success)
	}
	// A restarted process sees the same durable terminal result, not a
	// process-owned mutex snapshot. Same job/decision is idempotent.
	winner := verified
	if success.jobID == "cas-worker-rejects" {
		winner = rejected
	}
	got, err := b.CommitServerDecision(ctx, attempt, success.jobID, winner)
	if err != nil || got != success.verdict {
		t.Fatalf("restarted/retried worker lost immutable decision: %+v %v", got, err)
	}
}

func TestPostgresVerdictGlobalJobUniquenessAndIdentityFencing(t *testing.T) {
	a, b, db, ctx := postgresAttestationTestStores(t)
	first := postgresAttestationAttempt("same-job-a")
	second := postgresAttestationAttempt("same-job-b")
	removePostgresAttestationTestRow(t, db, first.Identity.RunID)
	removePostgresAttestationTestRow(t, db, second.Identity.RunID)
	if _, err := a.CreatePending(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreatePending(ctx, second); err != nil {
		t.Fatal(err)
	}
	jobID := fmt.Sprintf("unique-job-%d", time.Now().UnixNano())
	firstDecision := Decision{Identity: first.Identity, Status: Rejected,
		RejectionReason: "invalid_trace"}
	if _, err := a.CommitServerDecision(ctx, first, jobID, firstDecision); err != nil {
		t.Fatal(err)
	}
	secondDecision := Decision{Identity: second.Identity, Status: Rejected,
		RejectionReason: "unsupported_ir"}
	if _, err := b.CommitServerDecision(ctx, second, jobID, secondDecision); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("globally reused replay job must reject: %v", err)
	}
	if v, err := b.CreatePending(ctx, second); err != nil || v.Status != Pending {
		t.Fatalf("conflicting replay job corrupted pending record: %+v %v", v, err)
	}
	mutated := second
	mutated.AuthScopeSHA256 = strings.Repeat("f", 64)
	if _, err := b.CreatePending(ctx, mutated); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("auth scope may not alias an existing run: %v", err)
	}
	if _, err := b.CommitServerDecision(ctx, mutated, "new-worker", secondDecision); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("different authorization scope finalized another attempt: %v", err)
	}
	mutated = second
	mutated.IssuedAt = mutated.IssuedAt.Add(time.Microsecond)
	if _, err := a.CreatePending(ctx, mutated); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("issuedAt cannot be revised on retry: %v", err)
	}
	mutated = second
	mutated.Identity.Seed++
	if _, err := b.CommitServerDecision(ctx, mutated, "new-worker", secondDecision); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("decision identity must match full immutable seed: %v", err)
	}
	if _, err := b.CommitServerDecision(ctx, second, "new-worker", secondDecision); err != nil {
		t.Fatalf("distinct replay job must finalize untouched pending run: %v", err)
	}
	if _, err := b.CommitServerDecision(ctx, second, "different-worker", secondDecision); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("same verdict under different replay job is not idempotent: %v", err)
	}
}

func TestPostgresVerdictMigrationAndRollbackPreserveImmutableRows(t *testing.T) {
	a, _, db, ctx := postgresAttestationTestStores(t)
	attempt := postgresAttestationAttempt("rollback")
	removePostgresAttestationTestRow(t, db, attempt.Identity.RunID)
	if _, err := a.CreatePending(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(ctx); err != nil {
		t.Fatalf("repeated migration may not reset history: %v", err)
	}
	// Simulate a transaction that updated all fields but crashed before
	// COMMIT: the atomic row should remain pending and re-playable.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `
	UPDATE runtime_attestation_verdict_v1 SET status='verified',
	final_job_id='rollback-lost-worker', score=11, state_sha256=$2
	WHERE run_id=$1`, attempt.Identity.RunID, strings.Repeat("e", 64))
	if err != nil {
		tx.Rollback()
		t.Fatalf("valid in-transaction candidate rejected: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if v, err := a.CreatePending(ctx, attempt); err != nil || v.Status != Pending {
		t.Fatalf("rolled-back work minted a verdict: %+v %v", v, err)
	}
	// Direct SQL cannot rewrite server-issued identity after publication.
	_, err = db.ExecContext(ctx, `UPDATE runtime_attestation_verdict_v1
	SET ir_sha256=$2 WHERE run_id=$1`,
		attempt.Identity.RunID, strings.Repeat("f", 64))
	if err == nil {
		t.Fatal("database allowed mutation of immutable full IR digest")
	}
	decision := Decision{Identity: attempt.Identity, Status: Verified, Score: 11,
		StateSHA256: strings.Repeat("e", 64)}
	verified, err := a.CommitServerDecision(ctx, attempt, "committed-worker", decision)
	if err != nil || verified.Status != Verified {
		t.Fatalf("rollback blocked subsequent legal finalization: %+v %v", verified, err)
	}
	_, err = db.ExecContext(ctx, `UPDATE runtime_attestation_verdict_v1
	SET status='pending', final_job_id=NULL, score=0, state_sha256=''
	WHERE run_id=$1`, attempt.Identity.RunID)
	if err == nil {
		t.Fatal("database allowed rollback or overwrite of terminal verdict")
	}
	if got, err := a.CreatePending(ctx, attempt); err != nil || got != verified {
		t.Fatalf("repeated migration or failed direct SQL altered verdict: %+v %v", got, err)
	}
}
