package attestation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLocalVerdictJournalPersistsPendingFinalAndExactJobIdempotenceAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server-only-verdicts.json")
	journal, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	id := policyIdentity()
	pending, err := journal.CreatePending(id)
	if err != nil || pending.Status != Pending {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	if existing, err := journal.CreatePending(id); err != nil || existing != pending {
		t.Fatalf("same server attempt should be idempotent: %+v %v", existing, err)
	}
	different := id
	different.Seed++
	if _, err := journal.CreatePending(different); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("reused run ID with foreign seed accepted: %v", err)
	}
	decision := Decision{
		Identity: id, Status: Verified, Score: 42, Won: true,
		StateSHA256: strings.Repeat("d", 64),
	}
	final, err := journal.CommitServerDecision("worker-job-1", decision)
	if err != nil || final.Status != Verified || final.Score != 42 {
		t.Fatalf("server final: %+v %v", final, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("verdict file must be private 0600: %v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ActualScore") || strings.Contains(string(data), "client_verified") {
		t.Fatal("browser-owned legacy verdicts cannot leak into server-only journal")
	}
	reloaded, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatalf("restart load failed: %v", err)
	}
	if persisted, ok := reloaded.Read(id.RunID); !ok || persisted != final {
		t.Fatalf("trusted outcome lost across restart: %+v %v", persisted, ok)
	}
	for i := 0; i < 100; i++ {
		got, err := reloaded.CommitServerDecision("worker-job-1", decision)
		if err != nil || got != final {
			t.Fatalf("retry %d changed final: %+v %v", i, got, err)
		}
	}
	if _, err := reloaded.CommitServerDecision("worker-job-2", decision); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("different worker reused same score: %v", err)
	}
	differentScore := decision
	differentScore.Score = 43
	if _, err := reloaded.CommitServerDecision("worker-job-1", differentScore); !errors.Is(err, ErrConflictingVerdict) {
		t.Fatalf("same worker altered accepted score: %v", err)
	}
	foreign := decision
	foreign.Identity.TraceSHA256 = strings.Repeat("f", 64)
	if _, err := reloaded.CommitServerDecision("worker-job-1", foreign); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("foreign trace overwrote trusted verdict: %v", err)
	}
	if got, ok := reloaded.Read(id.RunID); !ok || got != final {
		t.Fatalf("bad writes changed known verdict: %+v %v", got, ok)
	}
}

func TestLocalVerdictJournalRejectsForgedPendingAndRepeatedRejectedDecision(t *testing.T) {
	journal, err := OpenLocalVerdictJournal(filepath.Join(t.TempDir(), "verdict.json"))
	if err != nil {
		t.Fatal(err)
	}
	id := policyIdentity()
	foreign := id
	foreign.IRSHA256 = "forged-short-hash"
	if _, err := journal.CreatePending(foreign); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("unbound full game digest accepted: %v", err)
	}
	pending, err := journal.CreatePending(id)
	if err != nil || pending.Status != Pending {
		t.Fatal(err)
	}
	if _, err := journal.CommitServerDecision("invalid/job", Decision{
		Identity: id, Status: Verified,
		StateSHA256: strings.Repeat("a", 64),
	}); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("unbounded job accepted: %v", err)
	}
	reject := Decision{Identity: id, Status: Rejected, RejectionReason: "unsupported_ir"}
	got, err := journal.CommitServerDecision("replay-worker-1", reject)
	if err != nil || got.Status != Rejected || got.Score != 0 || got.Won {
		t.Fatalf("unsupported IR must fail closed without score: %+v %v", got, err)
	}
	if x, err := journal.CommitServerDecision("replay-worker-1", reject); err != nil || x != got {
		t.Fatalf("same final reject should be idempotent: %+v %v", x, err)
	}
	if _, err := journal.CommitServerDecision("replay-worker-2", reject); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("second worker may double-consume reject: %v", err)
	}
	promote := Decision{Identity: id, Status: Verified, Score: 9, StateSHA256: strings.Repeat("a", 64)}
	if _, err := journal.CommitServerDecision("replay-worker-1", promote); !errors.Is(err, ErrConflictingVerdict) {
		t.Fatalf("rejected client attempt illegally promoted: %v", err)
	}
}

func TestLocalVerdictJournalRejectsCorruptAndWorldReadableSnapshots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server-only.json")
	j, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := j.CreatePending(policyIdentity()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Recompute valid outer JSON with an altered snapshot but *unchanged*
	// digest: it must not load, even if the payload remains parseable.
	var env localJournalEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	env.Snapshot = append([]byte(nil), env.Snapshot...)
	for i := range env.Snapshot {
		if env.Snapshot[i] == 'p' {
			env.Snapshot[i] = 'q'
			break
		}
	}
	corrupted, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, corrupted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocalVerdictJournal(path); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("tampered snapshot reopened: %v", err)
	}
	if err := os.WriteFile(path, data[:len(data)/2], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocalVerdictJournal(path); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("truncated trusted scores silently reset: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocalVerdictJournal(path); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("world-readable player identities silently admitted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "unsafe"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocalVerdictJournal(path); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("symlink journal accepted: %v", err)
	}
}

func TestLocalVerdictJournalFailedWriteLeavesNoInMemoryVerdict(t *testing.T) {
	dir := t.TempDir()
	blocking := filepath.Join(dir, "file-not-directory")
	path := filepath.Join(blocking, "verdicts.json")
	// Open a fresh missing journal before the parent path is poisoned:
	// the first pending write must fail, not silently update memory.
	journal, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocking, []byte("blocking"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.CreatePending(policyIdentity()); !errors.Is(err, ErrJournalUnavailable) {
		t.Fatalf("write to unavailable parent claimed success: %v", err)
	}
	if got, ok := journal.Read(policyIdentity().RunID); ok || got != (Verdict{}) {
		t.Fatalf("failed write mutated memory: %+v %v", got, ok)
	}
	if _, err := journal.CommitServerDecision("job-1", Decision{
		Identity: policyIdentity(), Status: Verified, StateSHA256: strings.Repeat("a", 64),
	}); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("no persisted pending may be verified: %v", err)
	}
}

func TestLocalVerdictJournalIsSerializedAcross100ConcurrentRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdicts.json")
	journal, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	id := policyIdentity()
	const workers = 100
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := journal.CreatePending(id); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent pending conflict: %v", err)
	}
	decision := Decision{
		Identity: id, Status: Verified, Score: 7,
		StateSHA256: strings.Repeat("e", 64),
	}
	errs = make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := journal.CommitServerDecision("single-authorized-worker", decision); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent final retry conflict: %v", err)
	}
	reloaded, err := OpenLocalVerdictJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reloaded.Read(id.RunID); !ok || got.Score != 7 || got.Status != Verified {
		t.Fatalf("racing worker retries changed durable result: %+v %v", got, ok)
	}
}
