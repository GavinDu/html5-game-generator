package attestation

import (
	"errors"
	"fmt"

	"doodlequest/internal/gameir"
)

// errStartScoreNotAdmitted denotes unsupported or invalid published Game IR.
// It must never be converted into a successful verdict or a false predicate.
var errStartScoreNotAdmitted = errors.New("Game IR not admitted for diagnostic start-score reduction")

// startScoreStepV1 is a deterministic, untrusted diagnostic transition.
// In particular, it carries no verified or score-attestation status.
type startScoreStepV1 struct {
	RuleID      string
	ActionIndex int
	ScoreAfter  int64
}

type startScoreResultV1 struct {
	Capability string
	Score      int64
	Steps      []startScoreStepV1
}

// reduceStartScoreV1 runs only the start-only subset already admitted by the
// private strict Game IR profile. It does not consume client-reported scores,
// event streams, seeds or scripts. The real Pixi runtime emits game_start once
// on non-shell v1 boot; this helper executes admitted rules in their authored
// order. Actual Pixi-vs-Go browser conformance is a separate issue (#469).
func reduceStartScoreV1(doc *gameir.Document) (startScoreResultV1, error) {
	admission := supportedPublishedIRProfile(doc)
	if admission.Kind != replayIRAdmissible || admission.Capability != replayStartScoreProfileV1 {
		return startScoreResultV1{}, fmt.Errorf("%w: %s", errStartScoreNotAdmitted, admission.Reason)
	}

	result := startScoreResultV1{
		Capability: replayStartScoreProfileV1,
		Steps:      make([]startScoreStepV1, 0, admission.MaxActions),
	}
	for _, rule := range doc.Rules {
		for index, action := range rule.Actions {
			value, ok := startScoreLiteral(action.Value)
			if !ok {
				// Defensive after admission: never treat a non-integer rule
				// as zero or silently skip an authored score mutation.
				return startScoreResultV1{}, fmt.Errorf("%w: non_integer_score_literal", errStartScoreNotAdmitted)
			}
			result.Score += value
			result.Steps = append(result.Steps, startScoreStepV1{
				RuleID:      rule.ID,
				ActionIndex: index,
				ScoreAfter:  result.Score,
			})
		}
	}
	return result, nil
}
