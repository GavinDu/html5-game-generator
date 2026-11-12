package gameir

import (
	"fmt"
	"strings"
)

// The experience block (IR 1.2) — the platform shell contract. Content
// stays the creator's; how a game is organized and controlled is
// standardized here: one archetype from the closed catalog, one named
// control scheme, a declared objective and failure model, a pacing
// ramp as data, and presentation slots the runtime shell renders
// identically in every game. See game-ir-1.2-fused-design.md.

type Experience struct {
	Archetype        string            `json:"archetype"`
	Controls         ExpControls       `json:"controls"`
	ChaserGap        *ExpChaserGap     `json:"chaserGap,omitempty"`
	Objective        ExpObjective      `json:"objective"`
	HUD              ExpHUD            `json:"hud,omitempty"`
	Start            ExpStart          `json:"start,omitempty"`
	Failure          ExpFailure        `json:"failure,omitempty"`
	Pacing           ExpPacing         `json:"pacing,omitempty"`
	Milestones       []float64         `json:"milestones,omitempty"`
	MilestoneVisuals map[string]string `json:"milestoneVisuals,omitempty"`
	Result           ExpResult         `json:"result,omitempty"`
	Leaderboard      ExpLeaderboard    `json:"leaderboard,omitempty"`
	Feedback         ExpFeedback       `json:"feedback,omitempty"`
}

type ExpControls struct {
	Scheme       string `json:"scheme"`
	Hint         string `json:"hint,omitempty"`
	MovementAxes string `json:"movementAxes,omitempty"` // input.movement_axes.v1: x | xy
}

type ExpChaserGap struct {
	Start             float64 `json:"start"`
	Max               float64 `json:"max"`
	CaughtAt          float64 `json:"caughtAt"`
	RecoveryPerSecond float64 `json:"recoveryPerSecond"`
}

type ExpObjective struct {
	Kind             string `json:"kind"`
	Target           int    `json:"target,omitempty"`
	Label            string `json:"label"`
	ProgressVariable string `json:"progressVariable,omitempty"`
}

type ExpHUD struct {
	Left  string `json:"left,omitempty"`
	Right string `json:"right,omitempty"`
}

type ExpStart struct {
	Mode string `json:"mode,omitempty"` // tap_to_start (default) | auto
}

type ExpFailure struct {
	Mode           string `json:"mode,omitempty"` // health (default) | gentle | sudden-death | none (= gentle, declared)
	Health         int    `json:"health,omitempty"`
	InvulnerableMs int    `json:"invulnerableMs,omitempty"`
	MissCost       string `json:"missCost,omitempty"`
}

type ExpPacing struct {
	TargetSessionSec     int           `json:"targetSessionSec,omitempty"`
	FirstRewardWithinSec int           `json:"firstRewardWithinSec,omitempty"`
	ExpectedCatchRate    float64       `json:"expectedCatchRate,omitempty"`
	Ramp                 []ExpRampStep `json:"ramp,omitempty"`
}

type ExpRampStep struct {
	At              float64 `json:"at"` // normalized session time 0..1
	SpawnIntervalMs int     `json:"spawnIntervalMs,omitempty"`
	SpeedMul        float64 `json:"speedMul,omitempty"`
}

type ExpResult struct {
	Win  ExpResultSide `json:"win,omitempty"`
	Lose ExpResultSide `json:"lose,omitempty"`
}

type ExpResultSide struct {
	Title string    `json:"title,omitempty"`
	Stars *ExpStars `json:"stars,omitempty"`
}

type ExpStars struct {
	By            string `json:"by,omitempty"` // time (default) | score
	ThresholdsSec []int  `json:"thresholdsSec,omitempty"`
}

type ExpLeaderboard struct {
	Metric string `json:"metric,omitempty"` // score (default) | time | streak
	Order  string `json:"order,omitempty"`  // desc (default) | asc
}

type ExpFeedback struct {
	Haptics string `json:"haptics,omitempty"` // none (default) | standard — native bridge decision pending
	Juice   string `json:"juice,omitempty"`   // standard (default) | rich | none
}

var (
	expArchetypes = enum("catcher", "dodger", "tapper", "runner", "physics", "puzzle")
	// Schemes are the control gestures the character controller really
	// performs; swipe and stick_button were removed 2026-10-23 with the
	// rest of the unimplemented vocabulary (see ir.go assetKinds).
	expSchemes    = enum("drag_x", "drag_free", "tap", "tap_target", "lane_tap")
	expObjectives = enum("collect", "survive", "reach", "score")
	expFailModes  = enum("health", "gentle", "sudden-death", "none")
	expHUDSlots   = enum("objective_progress", "score", "health", "timer", "none")
)

// validateExperience enforces the block's structural contract. It is
// called for every document: a 1.2 document MUST carry the block and
// the shell.v1 capability; earlier versions MAY carry it (validated
// the same way when present).
func validateExperience(d *Document) error {
	exp := d.Experience
	if exp == nil {
		if d.IRVersion == Version12 {
			return fmt.Errorf("irVersion 1.2 requires an experience block")
		}
		// A v1.0/v1.1 document cannot advertise the movement-plane
		// capability without the experience it would configure.
		for _, c := range d.Capabilities {
			if c == "input.movement_axes.v1" {
				return fmt.Errorf("input.movement_axes.v1 requires Game IR 1.2 and experience")
			}
		}
		return nil
	}
	shellCap := false
	for _, c := range d.Capabilities {
		if c == "shell.v1" {
			shellCap = true
		}
	}
	if !shellCap {
		return fmt.Errorf("experience block requires the shell.v1 capability")
	}
	if !has(expArchetypes, exp.Archetype) {
		return fmt.Errorf("experience.archetype %q is not in the catalog", exp.Archetype)
	}
	if !has(expSchemes, exp.Controls.Scheme) {
		return fmt.Errorf("experience.controls.scheme %q is not a shell scheme", exp.Controls.Scheme)
	}
	if len(exp.Controls.Hint) > 80 {
		return fmt.Errorf("experience.controls.hint exceeds 80 characters")
	}
	// Versioned, declarative movement-axis semantics. Published 1.2 games
	// without this capability retain their exact legacy movement/replay
	// behavior; a newer compiler must explicitly declare an axis choice.
	axesCap := false
	for _, c := range d.Capabilities {
		if c == "input.movement_axes.v1" {
			axesCap = true
			break
		}
	}
	axes := exp.Controls.MovementAxes
	if axes != "" && !axesCap {
		return fmt.Errorf("experience.controls.movementAxes requires input.movement_axes.v1 capability")
	}
	if axesCap {
		if d.IRVersion != Version12 {
			return fmt.Errorf("input.movement_axes.v1 requires Game IR 1.2")
		}
		if axes != "x" && axes != "xy" {
			return fmt.Errorf("experience.controls.movementAxes must be x or xy")
		}
		switch exp.Controls.Scheme {
		case "lane_tap", "drag_x":
			if axes != "x" {
				return fmt.Errorf("experience.controls.movementAxes must be x for %s", exp.Controls.Scheme)
			}
		case "drag_free":
			if axes != "xy" {
				return fmt.Errorf("experience.controls.movementAxes must be xy for drag_free")
			}
		case "tap_target":
			return fmt.Errorf("tap_target targets entities and cannot declare player movementAxes")
		}
	}
	if gap := exp.ChaserGap; gap != nil {
		if gap.Start <= 0 || gap.Max <= gap.Start || gap.CaughtAt < 0 || gap.CaughtAt >= gap.Start {
			return fmt.Errorf("experience.chaserGap requires 0 <= caughtAt < start < max")
		}
		if gap.Max > 10000 || gap.RecoveryPerSecond < 0 || gap.RecoveryPerSecond > 100 {
			return fmt.Errorf("experience.chaserGap max or recoveryPerSecond out of range")
		}
		if exp.Controls.Scheme != "lane_tap" {
			return fmt.Errorf("experience.chaserGap requires lane_tap controls")
		}
	}
	if !has(expObjectives, exp.Objective.Kind) {
		return fmt.Errorf("experience.objective.kind %q unsupported", exp.Objective.Kind)
	}
	if exp.Objective.Kind == "collect" && exp.Objective.Target < 1 {
		return fmt.Errorf("experience.objective.target must be >= 1 for a collect objective")
	}
	if strings.TrimSpace(exp.Objective.Label) == "" || len(exp.Objective.Label) > 60 {
		return fmt.Errorf("experience.objective.label is required (max 60 characters)")
	}
	if len(exp.Objective.ProgressVariable) > 80 {
		return fmt.Errorf("experience.objective.progressVariable exceeds 80 characters")
	}
	if exp.Objective.ProgressVariable != "" && exp.Objective.Kind != "collect" {
		return fmt.Errorf("experience.objective.progressVariable is only supported for collect objectives")
	}
	for _, slot := range []string{exp.HUD.Left, exp.HUD.Right} {
		if slot != "" && !has(expHUDSlots, slot) {
			return fmt.Errorf("experience.hud slot %q unsupported", slot)
		}
	}
	if exp.Start.Mode != "" && exp.Start.Mode != "tap_to_start" && exp.Start.Mode != "auto" {
		return fmt.Errorf("experience.start.mode %q unsupported", exp.Start.Mode)
	}
	fm := exp.Failure.Mode
	if fm != "" && !has(expFailModes, fm) {
		return fmt.Errorf("experience.failure.mode %q unsupported", fm)
	}
	if exp.Failure.Health != 0 && (exp.Failure.Health < 1 || exp.Failure.Health > 9) {
		return fmt.Errorf("experience.failure.health must be between 1 and 9")
	}
	if exp.Failure.InvulnerableMs < 0 || exp.Failure.InvulnerableMs > 5000 {
		return fmt.Errorf("experience.failure.invulnerableMs must be between 0 and 5000")
	}
	p := exp.Pacing
	if p.TargetSessionSec != 0 && (p.TargetSessionSec < 10 || p.TargetSessionSec > 1200) {
		return fmt.Errorf("experience.pacing.targetSessionSec must be between 10 and 1200")
	}
	if p.FirstRewardWithinSec != 0 && (p.FirstRewardWithinSec < 1 || p.FirstRewardWithinSec > 60) {
		return fmt.Errorf("experience.pacing.firstRewardWithinSec must be between 1 and 60")
	}
	if p.ExpectedCatchRate != 0 && (p.ExpectedCatchRate <= 0 || p.ExpectedCatchRate > 1) {
		return fmt.Errorf("experience.pacing.expectedCatchRate must be in (0, 1]")
	}
	prevAt := -1.0
	for i, step := range p.Ramp {
		if step.At < 0 || step.At > 1 {
			return fmt.Errorf("experience.pacing.ramp[%d].at must be in [0, 1]", i)
		}
		if step.At <= prevAt {
			return fmt.Errorf("experience.pacing.ramp must be strictly increasing in at")
		}
		prevAt = step.At
		if step.SpawnIntervalMs != 0 && (step.SpawnIntervalMs < 100 || step.SpawnIntervalMs > 10000) {
			return fmt.Errorf("experience.pacing.ramp[%d].spawnIntervalMs must be between 100 and 10000", i)
		}
		if step.SpeedMul != 0 && (step.SpeedMul < 0.1 || step.SpeedMul > 5) {
			return fmt.Errorf("experience.pacing.ramp[%d].speedMul must be between 0.1 and 5", i)
		}
	}
	prevM := 0.0
	for i, m := range exp.Milestones {
		if m <= 0 || m > 1 {
			return fmt.Errorf("experience.milestones[%d] must be in (0, 1]", i)
		}
		if m <= prevM {
			return fmt.Errorf("experience.milestones must be strictly increasing")
		}
		prevM = m
	}
	for _, side := range []ExpResultSide{exp.Result.Win, exp.Result.Lose} {
		if len(side.Title) > 80 {
			return fmt.Errorf("experience.result title exceeds 80 characters")
		}
		if side.Stars != nil {
			if side.Stars.By != "" && side.Stars.By != "time" && side.Stars.By != "score" {
				return fmt.Errorf("experience.result stars.by %q unsupported", side.Stars.By)
			}
			th := side.Stars.ThresholdsSec
			if len(th) != 0 {
				if len(th) != 3 {
					return fmt.Errorf("experience.result stars.thresholdsSec must have exactly 3 entries")
				}
				if !(th[0] > th[1] && th[1] > th[2] && th[2] > 0) {
					return fmt.Errorf("experience.result stars.thresholdsSec must be positive and strictly descending (e.g. [60, 40, 25])")
				}
			}
		}
	}
	if exp.Leaderboard.Metric != "" && !has(enum("score", "time", "streak"), exp.Leaderboard.Metric) {
		return fmt.Errorf("experience.leaderboard.metric %q unsupported", exp.Leaderboard.Metric)
	}
	if exp.Leaderboard.Order != "" && exp.Leaderboard.Order != "asc" && exp.Leaderboard.Order != "desc" {
		return fmt.Errorf("experience.leaderboard.order %q unsupported", exp.Leaderboard.Order)
	}
	if exp.Feedback.Haptics != "" && !has(enum("none", "standard"), exp.Feedback.Haptics) {
		return fmt.Errorf("experience.feedback.haptics %q unsupported", exp.Feedback.Haptics)
	}
	if exp.Feedback.Juice != "" && !has(enum("none", "standard", "rich"), exp.Feedback.Juice) {
		return fmt.Errorf("experience.feedback.juice %q unsupported", exp.Feedback.Juice)
	}
	return validateExperienceOutcomes(d)
}

// validateExperienceOutcomes connects the declarative objective to the
// terminal rules the browser actually runs. A bonus score event must never
// finish a creator-authored "catch 12 hearts" game early. Likewise,
// gentle/no-fail drawings must not contain hidden losing end_game rules.
func validateExperienceOutcomes(d *Document) error {
	exp := d.Experience
	if exp == nil {
		return nil
	}
	progressVariable := strings.TrimSpace(exp.Objective.ProgressVariable)
	strictCollection := exp.Objective.Kind == "collect" && progressVariable != ""
	hasProgressAction := false
	hasObjectiveWin := false
	for _, rule := range d.Rules {
		for _, action := range rule.Actions {
			if strictCollection && (action.Type == "add_variable" || action.Type == "set_variable") &&
				action.Variable == progressVariable {
				n, ok := asNumber(action.Value)
				if ok && n > 0 {
					hasProgressAction = true
				}
			}
			if action.Type != "end_game" {
				continue
			}
			won, ok := action.Value.(bool)
			if !ok {
				return fmt.Errorf("rule %q end_game requires a boolean value, not a truthy string or number", rule.ID)
			}
			if !won && (exp.Failure.Mode == "gentle" || exp.Failure.Mode == "none") {
				return fmt.Errorf("rule %q declares a loss in a %s no-fail experience", rule.ID, exp.Failure.Mode)
			}
			if !strictCollection || !won {
				continue
			}
			if rule.When.Event != "variable_changed" {
				return fmt.Errorf("rule %q wins via %q rather than collected objective variable %q", rule.ID, rule.When.Event, progressVariable)
			}
			match := false
			for _, condition := range rule.Conditions {
				if condition.Variable != progressVariable ||
					(condition.Operator != "eq" && condition.Operator != "gte") {
					continue
				}
				threshold, numeric := asNumber(condition.Value)
				if numeric && threshold == float64(exp.Objective.Target) {
					match = true
				}
			}
			if !match {
				return fmt.Errorf("rule %q winning threshold must match the %d-count objective %q", rule.ID, exp.Objective.Target, progressVariable)
			}
			hasObjectiveWin = true
		}
	}
	if strictCollection {
		if !hasProgressAction {
			return fmt.Errorf("collect objective %q has no positive increment or assignment", progressVariable)
		}
		if !hasObjectiveWin {
			return fmt.Errorf("collect objective %q has no progress-gated winning rule", progressVariable)
		}
	}
	return nil
}
