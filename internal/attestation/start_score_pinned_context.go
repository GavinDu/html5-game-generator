package attestation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"doodlequest/internal/gameir"
)

const maxPinnedStartScoreIRBytes = 1 << 20

// pinnedStartScoreContext is private, pure diagnostic evidence for the narrow
// entityless game_start/add_score profile. Neither its digest nor its score
// authenticates a publisher, server-granted seed, browser run or final verdict.
type pinnedStartScoreContext struct {
	Profile             string
	RawIRSHA256         string
	TypedIRSHA256       string
	OrderedWorkSHA256   string
	TypedContextSHA256  string
	PinnedContextSHA256 string
	Seed                uint64
	DurationTicks       int
	Score               int64
}

// fingerprintPinnedStartScoreContext rejects any mismatch between the exact
// supplied Game IR bytes and the expected full SHA-256 of a future server-owned
// immutable published IR. The expected digest's provenance MUST be checked by
// the caller: this pure function has no authentication, database access or
// authority to mark a score verified.
//
// It does not execute user input, a physics tick, Pixi, or an arbitrary IR.
func fingerprintPinnedStartScoreContext(
	rawIR []byte, expectedPublishedIRSHA256 string,
	seed uint64, durationTicks int, inputEdges []string,
) (pinnedStartScoreContext, error) {
	if len(rawIR) == 0 || len(rawIR) > maxPinnedStartScoreIRBytes {
		return pinnedStartScoreContext{}, fmt.Errorf("pinned diagnostic IR byte budget exceeded")
	}
	if len(expectedPublishedIRSHA256) != 64 {
		return pinnedStartScoreContext{}, fmt.Errorf("expected published IR SHA-256 must be 64 lowercase hex")
	}
	for i := 0; i < len(expectedPublishedIRSHA256); i++ {
		ch := expectedPublishedIRSHA256[i]
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return pinnedStartScoreContext{}, fmt.Errorf("expected published IR SHA-256 must be 64 lowercase hex")
		}
	}
	rawDigest := sha256.Sum256(rawIR)
	rawSHA := hex.EncodeToString(rawDigest[:])
	if rawSHA != expectedPublishedIRSHA256 {
		return pinnedStartScoreContext{}, fmt.Errorf("pinned diagnostic raw IR digest mismatch")
	}

	// Reuse the only supported, fail-closed scalar diagnostic profile. An
	// opaque but syntactically correct SHA cannot grant unsupported semantics.
	doc, err := gameir.DecodeAndValidate(rawIR)
	if err != nil {
		return pinnedStartScoreContext{}, fmt.Errorf("invalid pinned diagnostic Game IR: %w", err)
	}
	typed, err := fingerprintStartScoreContext(doc, seed, durationTicks, inputEdges)
	if err != nil {
		return pinnedStartScoreContext{}, fmt.Errorf("unsupported pinned diagnostic context: %w", err)
	}
	digest := sha256.New()
	// Domain-separated, delimiter-stable ASCII serialization with fixed
	// 64-hex digests and decimal integers. Version change is mandatory
	// before changing the grammar; a typed canonical hash alone is not raw
	// published-byte evidence.
	_, _ = fmt.Fprintf(digest, "go-scalar-pinned-start-score-v1\n%s\n%s\n%d\n%d\n",
		rawSHA, typed.ContextSHA256, seed, durationTicks)
	return pinnedStartScoreContext{
		Profile: "go-scalar-pinned-start-score-v1",
		RawIRSHA256: rawSHA,
		TypedIRSHA256: typed.CanonicalIRSHA256,
		OrderedWorkSHA256: typed.OrderedWorkSHA256,
		TypedContextSHA256: typed.ContextSHA256,
		PinnedContextSHA256: hex.EncodeToString(digest.Sum(nil)),
		Seed: seed, DurationTicks: durationTicks, Score: typed.Score,
	}, nil
}
