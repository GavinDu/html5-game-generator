package attestation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"doodlequest/internal/gameir"
)

// startScoreDiagnostic is private, deterministic source-conformance evidence.
// It is NOT a server-issued session, independently verified score or rank.
type startScoreDiagnostic struct {
	Profile        string
	Score          int64
	AppliedActions int
	TraceSHA256    string
}

// reduceStartScoreDiagnostic runs the explicitly admitted pre-tick game_start
// rule/action sequence in authored order. It does NOT process runtime ticks,
// event queues, native controls, player results, a win, or network requests.
func reduceStartScoreDiagnostic(doc *gameir.Document) (startScoreDiagnostic, error) {
	admission := supportedPublishedIRProfile(doc)
	if admission.Kind != replayIRAdmissible ||
		admission.Capability != replayStartScoreProfileV1 {
		return startScoreDiagnostic{}, fmt.Errorf(
			"replay diagnostic not admissible: %s:%s",
			admission.Kind, admission.Reason,
		)
	}

	// The fingerprint is an explicitly versioned, ASCII-only trace over
	// every action application, not merely the final (commutative) score.
	// The published Game IR validator bounds identifiers to ASCII IDs;
	// indices preserve authored rule/action order instead of sorting.
	digest := sha256.New()
	_, _ = fmt.Fprint(digest, replayStartScoreProfileV1, "\n")
	out := startScoreDiagnostic{Profile: replayStartScoreProfileV1}
	for ruleIndex, rule := range doc.Rules {
		for actionIndex, action := range rule.Actions {
			delta, ok := startScoreLiteral(action.Value)
			if !ok {
				// Defense-in-depth against accidental future policy drift.
				return startScoreDiagnostic{}, fmt.Errorf(
					"non-integer admitted score: rule %d action %d", ruleIndex, actionIndex,
				)
			}
			out.Score += delta
			out.AppliedActions++
			_, _ = fmt.Fprintf(digest, "%d:%d:%s:%d:%d\n",
				ruleIndex, actionIndex, rule.ID, delta, out.Score)
		}
	}
	if out.AppliedActions != admission.MaxActions {
		return startScoreDiagnostic{}, fmt.Errorf(
			"replay diagnostic action budget drift: %d != %d",
			out.AppliedActions, admission.MaxActions,
		)
	}
	out.TraceSHA256 = hex.EncodeToString(digest.Sum(nil))
	return out, nil
}
