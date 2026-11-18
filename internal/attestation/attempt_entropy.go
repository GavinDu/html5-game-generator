package attestation

// This package-private source draws CSPRNG candidate values only. It does NOT
// authenticate a player, issue a replay grant, create a pending verdict, or
// authorize a score. Durable uniqueness and revocation are separate #461/#462.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
)

const (
	attemptEntropyBytes = 68 // 32 ID + independent 32 nonce + 4 seed
	attemptIDPrefix     = "att_"
)

var errAttemptEntropyUnavailable = errors.New("server replay attempt entropy unavailable")

// attemptEntropyCandidate is intentionally unexported. The nonce digest is
// only an opaque deduplication/binding value; it is NOT a credential, signed
// bearer or evidence that a client executed a game.
type attemptEntropyCandidate struct {
	RunID       string
	NonceSHA256 string
	Seed        uint32
}

// newProductionAttemptEntropy is the sole production entry point: callers
// cannot inject fake client-supplied randomness or repeatable test seeds.
func newProductionAttemptEntropy() (attemptEntropyCandidate, error) {
	return readAttemptEntropy(rand.Reader)
}

// readAttemptEntropy accepts a reader exclusively to inject strict failure
// fixtures in tests. Never wire it to an HTTP request or external call site.
func readAttemptEntropy(source io.Reader) (attemptEntropyCandidate, error) {
	if source == nil {
		return attemptEntropyCandidate{}, errAttemptEntropyUnavailable
	}
	var random [attemptEntropyBytes]byte
	defer clear(random[:])
	if _, err := io.ReadFull(source, random[:]); err != nil {
		return attemptEntropyCandidate{}, errAttemptEntropyUnavailable
	}
	// Reject broken entropy sources that returned an all-zero identifier,
	// all-zero nonce, or accidentally reused an identical 32-byte block.
	// These conditions would be astronomically unlikely with crypto/rand,
	// but deterministic/miswired production test readers must fail closed.
	var identifierZero, nonceZero bool = true, true
	for i := 0; i < 32; i++ {
		identifierZero = identifierZero && random[i] == 0
		nonceZero = nonceZero && random[32+i] == 0
	}
	if identifierZero || nonceZero {
		return attemptEntropyCandidate{}, errAttemptEntropyUnavailable
	}
	same := true
	for i := 0; i < 32; i++ {
		same = same && random[i] == random[32+i]
	}
	if same {
		return attemptEntropyCandidate{}, errAttemptEntropyUnavailable
	}
	digest := sha256.Sum256(random[32:64])
	return attemptEntropyCandidate{
		RunID:       attemptIDPrefix + hex.EncodeToString(random[0:32]),
		NonceSHA256: hex.EncodeToString(digest[:]),
		Seed:        binary.BigEndian.Uint32(random[64:68]),
	}, nil
}
