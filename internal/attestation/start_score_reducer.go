package attestation

import "doodlequest/internal/gameir"

// startScoreStep is a deterministic diagnostic state transition for the
// admitted minimal game_start/add_score IR profile. It is not a server-owned
// replay event, score truth, win verdict or leaderboard record.
type startScoreStep struct {
	Ordinal     int
	RuleID      string
	ActionIndex int
	Delta       int64
	Score       int64
}

// startScoreDiagnostic deliberately carries the fail-closed capability
// admission with every result, so callers cannot mistake zero for a valid
// score when an unsupported Game IR is rejected.
type startScoreDiagnostic struct {
	Admission replayIRAdmission
	Steps     []startScoreStep
	Score     int64
}

// reduceStartScoreDiagnostic runs precisely ONE game_start event over an
// immutable, validated Game IR. The preinstalled Pixi v1 runtime initializes
// score=0, walks rules and actions in authored order, and increments the
// score for explicit add_score values using Math.trunc. The only admitted
// literals are bounded exact signed integers; no rounding or float state.
// The function is private and has NO authority to issue verified results.
//
// Production callers must resolve an authenticated immutable published
// package independently; caller-supplied Go pointers are NOT trusted IR.
func reduceStartScoreDiagnostic(doc *gameir.Document) startScoreDiagnostic {
	admission := supportedPublishedIRProfile(doc)
	result := startScoreDiagnostic{Admission: admission}
	if admission.Kind != replayIRAdmissible {
		return result
	}

	// Allocate at most the capability's bounded action budget (8 * 8).
	steps := make([]startScoreStep, 0, admission.MaxActions)
	var score int64
	for _, rule := range doc.Rules {
		for actionIndex, action := range rule.Actions {
			delta, ok := startScoreLiteral(action.Value)
			if !ok || action.Type != "add_score" || len(steps) >= admission.MaxActions {
				// Mutated or untrusted state after admission must never yield
				// partial steps that can be confused with an accepted replay.
				return startScoreDiagnostic{
					Admission: unsupportedReplayIR("start_score_state_changed"),
				}
			}
			score += delta
			steps = append(steps, startScoreStep{
				Ordinal: len(steps), RuleID: rule.ID,
				ActionIndex: actionIndex, Delta: delta, Score: score,
			})
		}
	}
	if len(steps) != admission.MaxActions ||
		score < -64_000_000 || score > 64_000_000 {
		return startScoreDiagnostic{
			Admission: unsupportedReplayIR("start_score_work_budget_changed"),
		}
	}
	result.Steps = steps
	result.Score = score
	return result
}
