// Package semanticreplay contains server-owned, pure diagnostic semantics.
// It MUST NOT be treated as an attestation or a source of ranked scores.
package semanticreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"

	"doodlequest/internal/gameir"
	"doodlequest/internal/replaytrace"
)

const (
	ProofVersion = "diagnostic-input-score-tap-v1"
	maxRules = 32
	maxScoreMagnitude int64 = 1_000_000_000
	maxScoreDelta int64 = 1_000
	maxWork = 50_000
)

var (
	ErrInvalidIR = errors.New("semantic_diag_invalid_ir")
	ErrUnsupportedIR = errors.New("semantic_diag_unsupported_ir")
	ErrUnsupportedInput = errors.New("semantic_diag_unsupported_input")
	ErrUnsupportedRule = errors.New("semantic_diag_unsupported_rule")
	ErrUnsupportedAction = errors.New("semantic_diag_unsupported_action")
	ErrInvalidTrace = errors.New("semantic_diag_invalid_trace")
	ErrWorkBudget = errors.New("semantic_diag_work_budget")
	ErrScoreOverflow = errors.New("semantic_diag_score_overflow")
)

// SupportedIR is a narrow, explicitly non-general capability admission gate.
// Do not silently skip gameplay rules: a single unmodelled rule invalidates
// the entire diagnostic profile. The published Game IR remains declarative.
func SupportedIR(ir *gameir.Document) error {
	if ir == nil || gameir.Validate(ir) != nil {
		return ErrInvalidIR
	}
	if ir.IRVersion != gameir.Version || len(ir.Capabilities) != 0 ||
		ir.Experience != nil || ir.Game.SessionTargetSeconds != 0 {
		return ErrUnsupportedIR
	}
	if len(ir.Scenes) != 1 || len(ir.Assets) != 0 ||
		len(ir.Scenes[0].Entities) != 0 || len(ir.Scenes[0].Templates) != 0 ||
		len(ir.Scenes[0].Joints) != 0 || len(ir.Scenes[0].Emitters) != 0 ||
		len(ir.Scenes[0].Layers) != 0 || len(ir.Scenes[0].Gravity) != 0 ||
		len(ir.Scenes[0].WorldSize) != 0 || ir.Scenes[0].PixelsPerMeter != nil ||
		ir.Scenes[0].BackgroundAssetID != "" {
		return ErrUnsupportedIR
	}
	if len(ir.InputBindings) == 0 {
		return ErrUnsupportedInput
	}
	bindings := make(map[string]struct{}, len(ir.InputBindings))
	for _, b := range ir.InputBindings {
		if b.Input != "tap" || b.Key != "" {
			return ErrUnsupportedInput
		}
		if _, duplicate := bindings[b.Action]; duplicate {
			return ErrUnsupportedInput
		}
		bindings[b.Action] = struct{}{}
	}
	if len(ir.Rules) == 0 || len(ir.Rules) > maxRules {
		return ErrUnsupportedRule
	}
	for _, rule := range ir.Rules {
		if rule.When.Event != "input" || rule.When.InputAction == "" ||
			rule.When.Source != "" || rule.When.Target != "" ||
			rule.When.Value != nil || len(rule.Conditions) != 0 {
			return ErrUnsupportedRule
		}
		if _, known := bindings[rule.When.InputAction]; !known {
			return ErrUnsupportedRule
		}
		for _, action := range rule.Actions {
			if action.Target != "" || action.Variable != "" ||
				action.AssetID != "" || action.SceneID != "" || action.DurationMS != 0 {
				return ErrUnsupportedAction
			}
			switch action.Type {
			case "add_score":
				if _, ok := scoreDelta(action.Value); !ok {
					return ErrUnsupportedAction
				}
			case "end_game":
				if _, ok := action.Value.(bool); !ok {
					return ErrUnsupportedAction
				}
			default:
				return ErrUnsupportedAction
			}
		}
	}
	return nil
}

// Numeric values accepted by Go decoding are float64; author-written IR
// test values can also be integral Go types. Unlike the legacy browser
// coercions, the capability profile rejects fractional/string numbers.
func scoreDelta(raw any) (int64, bool) {
	var value float64
	switch v := raw.(type) {
	case float64:
		value = v
	case int:
		value = float64(v)
	case int64:
		value = float64(v)
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, false
		}
		value = n
	default:
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) ||
		math.Trunc(value) != value || math.Abs(value) > float64(maxScoreDelta) {
		return 0, false
	}
	return int64(value), true
}

// Diagnostic is not a server verdict: only a future authorized attestor may
// turn an independently verified run into a ranked score.
type Diagnostic struct {
	ProofVersion string `json:"proofVersion"`
	EvidenceStatus string `json:"evidenceStatus"`
	RankingScope string `json:"rankingScope"`
	Attested bool `json:"attested"`
	Seed uint32 `json:"seed"`
	DurationTicks uint32 `json:"durationTicks"`
	Score int64 `json:"score"`
	Terminal string `json:"terminal"`
	DownEvents uint32 `json:"downEvents"`
	ActionsApplied uint32 `json:"actionsApplied"`
	TraceSHA256 string `json:"traceSha256"`
	StateSHA256 string `json:"stateSha256"`
}

// ReplayDiagnostic consumes already typed v2 action edges in their admitted
// fixed-tick/ordinal order. Pointer release/cancel changes hold state in the
// shared strict validator; only an actual down executes an input rule.
// There are NO connections from this function to runs, rank or HTTP routes.
func ReplayDiagnostic(ir *gameir.Document, trace replaytrace.Trace, seed uint32) (Diagnostic, error) {
	var empty Diagnostic
	if err := SupportedIR(ir); err != nil {
		return empty, err
	}
	if err := replaytrace.Validate(ir, trace); err != nil {
		return empty, ErrInvalidTrace
	}
	irJSON, err := json.Marshal(ir)
	if err != nil {
		return empty, ErrInvalidIR
	}
	irDigest := sha256.Sum256(irJSON)
	traceDigest, err := replaytrace.CanonicalSHA256(ir, ir.Game.ID,
		hex.EncodeToString(irDigest[:]), seed, trace)
	if err != nil {
		return empty, ErrInvalidTrace
	}
	// No input: score remains zero, uncompleted game remains running.
	var score int64
	terminal := "running"
	var downEvents, applied, work uint32
	for _, edge := range trace.Events {
		if terminal != "running" {
			break
		}
		if edge.Phase != "down" {
			continue
		}
		downEvents++
		for _, rule := range ir.Rules {
			work++
			if work > maxWork {
				return empty, ErrWorkBudget
			}
			if rule.When.InputAction != edge.Action {
				continue
			}
			for _, action := range rule.Actions {
				work++
				if work > maxWork {
					return empty, ErrWorkBudget
				}
				switch action.Type {
				case "add_score":
					delta, _ := scoreDelta(action.Value) // gated by SupportedIR.
					if delta > 0 && score > maxScoreMagnitude-delta ||
						delta < 0 && score < -maxScoreMagnitude-delta {
						return empty, ErrScoreOverflow
					}
					score += delta
				case "end_game":
					if action.Value.(bool) {
						terminal = "win"
					} else {
						terminal = "lose"
					}
				}
				applied++
				if terminal != "running" {
					break
				}
			}
			if terminal != "running" {
				break
			}
		}
	}
	type resultFingerprint struct {
		Version string `json:"version"`
		IRSHA256 string `json:"irSha256"`
		TraceSHA256 string `json:"traceSha256"`
		Seed uint32 `json:"seed"`
		DurationTicks uint32 `json:"durationTicks"`
		Score int64 `json:"score"`
		Terminal string `json:"terminal"`
		DownEvents uint32 `json:"downEvents"`
		ActionsApplied uint32 `json:"actionsApplied"`
	}
	encoded, err := json.Marshal(resultFingerprint{
		ProofVersion, hex.EncodeToString(irDigest[:]), traceDigest,
		seed, trace.DurationTicks, score, terminal, downEvents, applied,
	})
	if err != nil {
		return empty, ErrInvalidTrace
	}
	state := sha256.Sum256(encoded)
	return Diagnostic{
		ProofVersion: ProofVersion, EvidenceStatus: "unverified",
		RankingScope: "none", Attested: false, Seed: seed,
		DurationTicks: trace.DurationTicks, Score: score, Terminal: terminal,
		DownEvents: downEvents, ActionsApplied: applied, TraceSHA256: traceDigest,
		StateSHA256: hex.EncodeToString(state[:]),
	}, nil
}
