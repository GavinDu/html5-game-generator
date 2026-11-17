package attestation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"doodlequest/internal/gameir"
)

// startScoreContextFingerprint is a private diagnostic identity for ONE strict
// no-input startup game profile. It is neither an attestation nor a game verdict.
type startScoreContextFingerprint struct {
	Profile           string
	CanonicalIRSHA256 string
	OrderedWorkSHA256 string
	ContextSHA256     string
	Seed              uint64
	DurationTicks     int
	Score             int64
}

// fingerprintStartScoreContext binds deterministic *diagnostic* work to the
// caller-specified seed and virtual horizon. Authorization of the published
// IR, the seed, and the input evidence belongs to a separate server-owned
// issuer/worker. Never use this output to promote a Verified score.
//
// The current admitted profile contains no input handlers. Any supplied input
// edge fails closed rather than being silently ignored; a duration in ticks
// does not authorize execution of a 60Hz replay worker.
func fingerprintStartScoreContext(
	doc *gameir.Document,
	seed uint64,
	durationTicks int,
	inputEdges []string,
) (startScoreContextFingerprint, error) {
	if durationTicks < 1 || durationTicks > 108_000 {
		return startScoreContextFingerprint{}, fmt.Errorf("diagnostic horizon outside 1..108000 ticks")
	}
	if len(inputEdges) != 0 {
		return startScoreContextFingerprint{}, fmt.Errorf("no-input startup profile rejects input edges")
	}
	work, err := reduceStartScoreDiagnostic(doc)
	if err != nil {
		return startScoreContextFingerprint{}, fmt.Errorf("unsupported diagnostic Game IR: %w", err)
	}

	// This is deterministic Go serialization of a validated typed Document,
	// NOT the original immutable published raw bytes, network authorization,
	// or a replacement for an authenticated full package IR SHA-256.
	canonical, err := json.Marshal(doc)
	if err != nil {
		return startScoreContextFingerprint{}, fmt.Errorf("serialize diagnostic Game IR: %w", err)
	}
	irDigest := sha256.Sum256(canonical)
	irSHA := hex.EncodeToString(irDigest[:])
	digest := sha256.New()
	// Versioned, ordered tuple with fixed-size hex hashes and decimal fields;
	// rule ID delimiters never enter this context transcript directly.
	_, _ = fmt.Fprintf(
		digest, "go-scalar-start-context-v1\n%s\n%s\n%d\n%d\n%d\n",
		irSHA, work.TraceSHA256, seed, durationTicks, work.Score,
	)
	return startScoreContextFingerprint{
		Profile:           "go-scalar-start-context-v1",
		CanonicalIRSHA256: irSHA,
		OrderedWorkSHA256: work.TraceSHA256,
		ContextSHA256:     hex.EncodeToString(digest.Sum(nil)),
		Seed:              seed,
		DurationTicks:     durationTicks,
		Score:             work.Score,
	}, nil
}
