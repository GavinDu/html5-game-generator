package attestation

import (
	"math"

	"doodlequest/internal/gameir"
)

// Diagnostic-only preflight capability: no input, renderer simulation, physics
// or independently trusted score. The actual replay worker must still execute
// every admitted rule against a server-owned package and seed.
const replayStartScoreProfileV1 = "go-scalar-start-score-v1"

type replayIRAdmissionKind string

const (
	replayIRInvalid     replayIRAdmissionKind = "invalid_game_ir"
	replayIRUnsupported replayIRAdmissionKind = "unsupported_replay_semantics"
	replayIRAdmissible  replayIRAdmissionKind = "diagnostic_subset_admissible"
)

type replayIRAdmission struct {
	Kind       replayIRAdmissionKind
	Reason     string
	Capability string
	MaxActions int
}

func unsupportedReplayIR(reason string) replayIRAdmission {
	return replayIRAdmission{Kind: replayIRUnsupported, Reason: reason}
}

// startScoreLiteral accepts only a bounded exact integer. PixiJS uses
// Math.trunc(action.value ?? 1) for add_score; we require an explicit numeric
// value and reject strings, booleans, fractions and oversize values.
func startScoreLiteral(raw any) (int64, bool) {
	const maximum = 1_000_000
	switch n := raw.(type) {
	case int:
		if n >= -maximum && n <= maximum {
			return int64(n), true
		}
	case int64:
		if n >= -maximum && n <= maximum {
			return n, true
		}
	case float64:
		if !math.IsNaN(n) && !math.IsInf(n, 0) && math.Trunc(n) == n &&
			n >= -maximum && n <= maximum {
			return int64(n), true
		}
	}
	return 0, false
}

// supportedPublishedIRProfile evaluates only independently parsed, immutable
// Game IR. It is NOT a publisher signature, user authorization, replay, score,
// or verdict. A true result is merely permission to TRY a future fixed-tick
// reducer with this capability; never write Verified from this preflight.
func supportedPublishedIRProfile(doc *gameir.Document) replayIRAdmission {
	if err := gameir.Validate(doc); err != nil {
		return replayIRAdmission{Kind: replayIRInvalid, Reason: "game_ir_validation_failed"}
	}
	if doc.IRVersion != gameir.Version || len(doc.Capabilities) != 0 ||
		doc.Experience != nil {
		return unsupportedReplayIR("ir_version_or_shell_capability")
	}
	// These fields affect shell/start behavior, runtime duration or selection
	// and have not been independently reproduced by the Go tick reducer.
	if doc.Game.Orientation != "any" || doc.Game.SessionTargetSeconds != 0 ||
		doc.Game.Audience != "" || doc.Game.Description != "" {
		return unsupportedReplayIR("game_configuration")
	}
	if doc.Metadata.CreatorDisplayName != "" || doc.Metadata.ParentGameID != "" ||
		doc.Metadata.ParentVersion != 0 || len(doc.Metadata.Tags) != 0 {
		return unsupportedReplayIR("metadata_outside_profile")
	}
	if len(doc.Assets) != 0 || len(doc.InputBindings) != 0 ||
		len(doc.Scenes) != 1 {
		return unsupportedReplayIR("assets_input_or_multiple_scenes")
	}
	scene := doc.Scenes[0]
	if scene.ID != doc.Game.EntryScene ||
		scene.BackgroundAssetID != "" || scene.BackgroundColor != "" ||
		len(scene.Gravity) != 0 || len(scene.WorldSize) != 0 ||
		scene.PixelsPerMeter != nil || len(scene.Layers) != 0 ||
		len(scene.Entities) != 0 || len(scene.Templates) != 0 ||
		len(scene.Joints) != 0 || len(scene.Emitters) != 0 {
		return unsupportedReplayIR("scene_has_world_or_dynamic_state")
	}
	// Pixi starts v1.0 without the optional shell and emits game_start once,
	// followed by scene_entered. An admitted rule may react to game_start
	// only; no input, scene-entered, timer, collision, threshold or recursion.
	if len(doc.Rules) == 0 || len(doc.Rules) > 8 {
		return unsupportedReplayIR("rule_count_outside_profile")
	}
	count := 0
	var totalScore int64
	for _, rule := range doc.Rules {
		if rule.When.Event != "game_start" || rule.When.Source != "" ||
			rule.When.Target != "" || rule.When.InputAction != "" ||
			rule.When.Value != nil || len(rule.Conditions) != 0 ||
			len(rule.Actions) == 0 || len(rule.Actions) > 8 {
			return unsupportedReplayIR("rule_event_or_conditions")
		}
		for _, action := range rule.Actions {
			if action.Type != "add_score" || action.Target != "" ||
				action.Variable != "" || action.AssetID != "" ||
				action.SceneID != "" || action.DurationMS != 0 {
				return unsupportedReplayIR("action_outside_start_score")
			}
			value, ok := startScoreLiteral(action.Value)
			if !ok {
				return unsupportedReplayIR("non_integer_score_literal")
			}
			totalScore += value
			count++
		}
	}
	// Stable integer score state stays exactly representable in JS Number.
	// Per-rule bounded work also stops a prolific authored start rule.
	if totalScore < -64_000_000 || totalScore > 64_000_000 {
		return unsupportedReplayIR("aggregate_score_budget")
	}
	return replayIRAdmission{
		Kind: replayIRAdmissible, Reason: "minimal_start_score_only",
		Capability: replayStartScoreProfileV1, MaxActions: count,
	}
}
