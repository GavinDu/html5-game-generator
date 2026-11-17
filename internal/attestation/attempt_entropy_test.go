package attestation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

type interruptedEntropyReader struct{ remaining int }

func (r *interruptedEntropyReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, errors.New("injected CSPRNG failure")
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 0x6e
	}
	r.remaining -= n
	return n, nil
}

func TestAttemptEntropyCandidateUsesDisjointOpaqueIDNonceAndFullSeed(t *testing.T) {
	source := make([]byte, attemptEntropyBytes)
	copy(source[:32], bytes.Repeat([]byte{0x11}, 32))
	copy(source[32:64], bytes.Repeat([]byte{0x22}, 32))
	copy(source[64:], []byte{0x12, 0x34, 0x56, 0x78})
	got, err := readAttemptEntropy(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	nonceDigest := sha256.Sum256(source[32:64])
	if got.RunID != "att_"+strings.Repeat("11", 32) ||
		got.NonceSHA256 != hex.EncodeToString(nonceDigest[:]) ||
		got.Seed != 0x12345678 {
		t.Fatalf("CSPRNG candidate identity or seed drifted: %+v", got)
	}
	if !validIdentifier(got.RunID) || !validSHA256(got.NonceSHA256) {
		t.Fatalf("invalid canonical ID/nonce binding shape: %+v", got)
	}
	if got.NonceSHA256 == got.RunID[4:] {
		t.Fatal("nonce block reused the run ID block")
	}
	// A candidate alone MUST NOT be a valid pending run: it lacks an
	// authenticated player, game, immutable package and trace evidence.
	if _, err := NewPending(Identity{RunID: got.RunID, Seed: got.Seed,
		ProofVersion: ProofVersion}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("raw entropy candidate mistakenly authorized replay: %v", err)
	}
}

func TestAttemptEntropyReaderFailuresNeverReturnPartialCandidate(t *testing.T) {
	valid := make([]byte, attemptEntropyBytes)
	copy(valid[:32], bytes.Repeat([]byte{0x11}, 32))
	copy(valid[32:64], bytes.Repeat([]byte{0x22}, 32))
	copy(valid[64:], []byte{1, 2, 3, 4})
	cases := map[string]io.Reader{
		"nil":               nil,
		"empty":             bytes.NewReader(nil),
		"short_one":         bytes.NewReader(valid[:1]),
		"short_67":          bytes.NewReader(valid[:67]),
		"partial_error":     &interruptedEntropyReader{remaining: 20},
		"zero_entire":       bytes.NewReader(make([]byte, attemptEntropyBytes)),
		"zero_identifier":   bytes.NewReader(append(make([]byte, 32), valid[32:]...)),
		"zero_nonce":        bytes.NewReader(append(append([]byte{}, valid[:32]...), make([]byte, 36)...)),
		"duplicated_blocks": bytes.NewReader(append(append([]byte{}, valid[:32]...), append(valid[:32], valid[64:]...)...)),
	}
	for name, reader := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := readAttemptEntropy(reader)
			if !errors.Is(err, errAttemptEntropyUnavailable) || got != (attemptEntropyCandidate{}) {
				t.Fatalf("bad/partial entropy admitted: %+v, %v", got, err)
			}
		})
	}
}

func TestProductionAttemptEntropyIsDistinctIn1024ConcurrentAllocations(t *testing.T) {
	const goroutines, perGoroutine = 32, 32
	values := make(chan attemptEntropyCandidate, goroutines*perGoroutine)
	errorsFromWorkers := make(chan error, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				got, err := newProductionAttemptEntropy()
				if err != nil {
					errorsFromWorkers <- err
					return
				}
				values <- got
			}
		}()
	}
	wg.Wait()
	close(values)
	close(errorsFromWorkers)
	for err := range errorsFromWorkers {
		t.Fatalf("production crypto/rand failed: %v", err)
	}
	ids, nonces, seeds := map[string]bool{}, map[string]bool{}, map[uint32]bool{}
	for value := range values {
		if !strings.HasPrefix(value.RunID, attemptIDPrefix) ||
			len(value.RunID) != len(attemptIDPrefix)+64 ||
			!validIdentifier(value.RunID) || !validSHA256(value.NonceSHA256) {
			t.Fatalf("noncanonical private attempt entropy: %+v", value)
		}
		if ids[value.RunID] || nonces[value.NonceSHA256] {
			t.Fatalf("repeated cryptographic candidate ID/nonce: %+v", value)
		}
		ids[value.RunID], nonces[value.NonceSHA256], seeds[value.Seed] = true, true, true
	}
	if len(ids) != goroutines*perGoroutine || len(nonces) != len(ids) || len(seeds) < 2 {
		t.Fatalf("incomplete or nonrandom candidates: %d IDs, %d nonces, %d unique seeds", len(ids), len(nonces), len(seeds))
	}
}
