// RequiredInputs is the step-2 design document: the AI's expansion of user
// uploads (videos, pictures, texts) into the predefined game required inputs
// (strategy, win/lose, background music, background video, characters/NPCs,
// features). It is the single upstream of GameSpec filling (engine routing)
// and RuntimeSpec IR generation.
//
// The JSON Schema is authoritative: required_inputs.schema.json in this
// package. These Go types mirror it; Validate enforces the structural
// constraints (required fields, closed enums, budget maximums) without a
// third-party schema library.
package gamespec

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// RequiredInputsVersion is the current GameDesignInput schema version.
// v1.1 adds the comedy-framework generative layer: premise (Judy Carter),
// strategy.twist (Greg Dean), and justified feature declarations (Li Dan).
const RequiredInputsVersion = "1.1"

type UploadSource struct {
	UploadID string `json:"uploadId"`
	Kind     string `json:"kind"` // image|video|text|audio
}

type InputSpec struct {
	Primary           string `json:"primary"`
	MaxDistinctInputs int    `json:"maxDistinctInputs,omitempty"`
}

type DifficultyInput struct {
	RampKind           string  `json:"rampKind"`
	RampStartSec       float64 `json:"rampStartSec,omitempty"`
	LevelCount         int     `json:"levelCount,omitempty"`
	Level1BotClearable bool    `json:"level1BotClearable,omitempty"`
}

type StrategyInput struct {
	Archetype   string          `json:"archetype"`
	CoreLoop    string          `json:"coreLoop"`
	Twist       TwistInput      `json:"twist"`
	Input       InputSpec       `json:"input"`
	SkillVsLuck string          `json:"skillVsLuck,omitempty"`
	LuckSources []string        `json:"luckSources,omitempty"`
	Difficulty  DifficultyInput `json:"difficulty"`
}

// TwistInput is the comedy-framework v1.1 mechanical surprise (Greg Dean's
// Target Assumption -> Reinterpretation): what the player expects from the
// setup versus what the game actually does. Every memorable mechanic is a
// reinterpretation; a game without a legible twist is a template with art.
type TwistInput struct {
	Assumption       string `json:"assumption"`
	Reinterpretation string `json:"reinterpretation"`
}

// PremiseInput is the comedy-framework v1.1 emotional core (Judy Carter's
// Hard Attitude): the stance of the game's premise, found in the source
// material before any mechanic is designed.
type PremiseInput struct {
	Attitude  string `json:"attitude"`  // strange|sucks|scary|silly
	Statement string `json:"statement"` // one line, max 280 chars
}

// FeatureDecl is one declared mechanic (powerup or mode) carrying its own
// justification — the Li Dan "mechanic diet": every mechanic must earn its
// place via LoopLink, and trace to the source material via SourceRef
// (authenticity: the design is an extension of the kid's drawing, not a
// generic template).
type FeatureDecl struct {
	Name      string `json:"name"`
	LoopLink  string `json:"loopLink"`
	SourceRef string `json:"sourceRef,omitempty"`
}

type WinSpec struct {
	Condition string `json:"condition"`
	Type      string `json:"type,omitempty"`
	Grading   string `json:"grading,omitempty"`
}

type LoseSpec struct {
	Condition string `json:"condition"`
}

type FailHandling struct {
	DeathKind string `json:"deathKind,omitempty"`
	HardFail  bool   `json:"hardFail"`
}

type WinLoseInput struct {
	Win          WinSpec      `json:"win"`
	Lose         LoseSpec     `json:"lose"`
	FailHandling FailHandling `json:"failHandling"`
	Scoring      string       `json:"scoring,omitempty"`
	Progression  string       `json:"progression,omitempty"`
}

type TutorialSpec struct {
	Kind          string `json:"kind,omitempty"`
	MandatoryText bool   `json:"mandatoryText"`
}

type RestartSpec struct {
	MaxSec           float64 `json:"maxSec,omitempty"`
	MaxClicks        int     `json:"maxClicks,omitempty"`
	NoMenuNavigation bool    `json:"noMenuNavigation,omitempty"`
}

type HookInput struct {
	TimeToInputSec       float64      `json:"timeToInputSec"`
	TimeToFirstRewardSec float64      `json:"timeToFirstRewardSec"`
	FirstRewardKind      string       `json:"firstRewardKind,omitempty"`
	Tutorial             TutorialSpec `json:"tutorial"`
	Restart              RestartSpec  `json:"restart"`
}

type MusicSpec struct {
	Mood  string `json:"mood,omitempty"`
	Style string `json:"style,omitempty"`
	Loop  bool   `json:"loop,omitempty"`
	Src   string `json:"src,omitempty"`
}

type AudioInput struct {
	Music                 MusicSpec `json:"music,omitempty"`
	Sfx                   []string  `json:"sfx,omitempty"`
	MuteToggle            bool      `json:"muteToggle,omitempty"`
	NoAutoplayBeforeInput bool      `json:"noAutoplayBeforeInput,omitempty"`
}

type BackgroundSpec struct {
	Kind           string `json:"kind,omitempty"`
	Description    string `json:"description,omitempty"`
	SourceUploadID string `json:"sourceUploadId,omitempty"`
}

type VisualInput struct {
	ArtStyle      string         `json:"artStyle,omitempty"`
	ArtPolicy     string         `json:"artPolicy,omitempty"`
	Background    BackgroundSpec `json:"background,omitempty"`
	Palette       []string       `json:"palette,omitempty"`
	EffectsBudget string         `json:"effectsBudget,omitempty"`
}

type CharacterSpec struct {
	Name       string   `json:"name"`
	Role       string   `json:"role,omitempty"` // enemy|friend|neutral|collectible (npcs)
	DesignRef  string   `json:"designRef"`
	Silhouette string   `json:"silhouette,omitempty"`
	Behaviors  []string `json:"behaviors,omitempty"` // RuntimeSpec behavior ids
}

type CharactersInput struct {
	Player CharacterSpec   `json:"player"`
	NPCs   []CharacterSpec `json:"npcs,omitempty"`
}

type CoopSpec struct {
	Enabled     bool   `json:"enabled"`
	InputScheme string `json:"inputScheme,omitempty"`
}

type FeaturesInput struct {
	Powerups    []FeatureDecl `json:"powerups,omitempty"`
	Modes       []FeatureDecl `json:"modes,omitempty"`
	Coop        CoopSpec      `json:"coop,omitempty"`
	Leaderboard bool          `json:"leaderboard,omitempty"`
	Skins       bool          `json:"skins,omitempty"`
}

type ConstraintsInput struct {
	SessionTargetSec int     `json:"sessionTargetSec"`
	PackageMB        float64 `json:"packageMB"`
	ColdStartSec     float64 `json:"coldStartSec"`
	Audience         string  `json:"audience,omitempty"`
	PerfBudget       string  `json:"perfBudget,omitempty"`
}

// RequiredInputs is the full step-2 design document.
type RequiredInputs struct {
	SchemaVersion string            `json:"schemaVersion"`
	WorkingTitle  string            `json:"workingTitle,omitempty"`
	Sources       []UploadSource    `json:"sources,omitempty"`
	Premise       PremiseInput      `json:"premise"`
	Strategy      StrategyInput     `json:"strategy"`
	WinLose       WinLoseInput      `json:"winLose"`
	Hook          HookInput         `json:"hook"`
	Audio         AudioInput        `json:"audio,omitempty"`
	Visual        VisualInput       `json:"visual,omitempty"`
	Characters    CharactersInput   `json:"characters"`
	Features      FeaturesInput     `json:"features,omitempty"`
	Plot          string            `json:"plot,omitempty"`
	Constraints   ConstraintsInput  `json:"constraints"`
	Provenance    map[string]string `json:"provenance,omitempty"`
}

// LoadRequiredInputs reads a GameDesignInput JSON file.
func LoadRequiredInputs(path string) (*RequiredInputs, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read required inputs: %w", err)
	}
	var ri RequiredInputs
	if err := json.Unmarshal(raw, &ri); err != nil {
		return nil, fmt.Errorf("parse required inputs: %w", err)
	}
	return &ri, nil
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func oneOf(path, v string, allowed ...string) error {
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	if v == "" {
		return nil // optional fields may be empty
	}
	return fmt.Errorf("%s: %q not in [%s]", path, v, strings.Join(allowed, ", "))
}

func reqOneOf(path, v string, allowed ...string) error {
	if v == "" {
		return fmt.Errorf("%s: required", path)
	}
	return oneOf(path, v, allowed...)
}

// Validate enforces the structural contract of the JSON Schema:
// required fields, closed enums, and budget maximums.
func (r *RequiredInputs) Validate() error {
	if r.SchemaVersion != RequiredInputsVersion {
		return fmt.Errorf("schemaVersion: want %q, got %q", RequiredInputsVersion, r.SchemaVersion)
	}
	// v1.1: premise — the emotional core (Judy Carter). The AI must find the
	// emotional truth in the source material before designing mechanics.
	if err := reqOneOf("premise.attitude", r.Premise.Attitude,
		"strange", "sucks", "scary", "silly"); err != nil {
		return err
	}
	if r.Premise.Statement == "" {
		return fmt.Errorf("premise.statement: required")
	}
	if n := len([]rune(r.Premise.Statement)); n > 280 {
		return fmt.Errorf("premise.statement: max 280 chars, got %d", n)
	}
	for i, s := range r.Sources {
		if s.UploadID == "" {
			return fmt.Errorf("sources[%d].uploadId: required", i)
		}
		if err := reqOneOf(fmt.Sprintf("sources[%d].kind", i), s.Kind, "image", "video", "text", "audio"); err != nil {
			return err
		}
	}
	st := r.Strategy
	if err := reqOneOf("strategy.archetype", st.Archetype,
		"catcher", "tapper", "dodger", "endless_runner",
		"one_input_physics", "physics_trials", "coop_puzzle_platformer",
		"abstract_puzzle", "territory_arena", "maze_arcade",
		"physics_sports", "action_platformer"); err != nil {
		return err
	}
	if st.CoreLoop == "" {
		return fmt.Errorf("strategy.coreLoop: required")
	}
	// v1.1: twist — the mechanical surprise (Greg Dean). The player expects
	// the assumption; the game delivers the reinterpretation.
	if st.Twist.Assumption == "" {
		return fmt.Errorf("strategy.twist.assumption: required")
	}
	if st.Twist.Reinterpretation == "" {
		return fmt.Errorf("strategy.twist.reinterpretation: required")
	}
	if err := reqOneOf("strategy.input.primary", st.Input.Primary,
		"tap", "drag", "tilt", "arrows", "wasd", "touch_hold", "fling"); err != nil {
		return err
	}
	if st.Input.MaxDistinctInputs > 2 {
		return fmt.Errorf("strategy.input.maxDistinctInputs: max 2, got %d", st.Input.MaxDistinctInputs)
	}
	if err := oneOf("strategy.skillVsLuck", st.SkillVsLuck, "skill_dominant", "mixed", "luck_dominant"); err != nil {
		return err
	}
	for i, ls := range st.LuckSources {
		if err := reqOneOf(fmt.Sprintf("strategy.luckSources[%d]", i), ls, "spawns", "drops", "physics_chaos", "none"); err != nil {
			return err
		}
	}
	if err := reqOneOf("strategy.difficulty.rampKind", st.Difficulty.RampKind,
		"speed_ramp", "hazard_escalation", "exponential_pressure", "level_arc", "none"); err != nil {
		return err
	}
	if st.Difficulty.LevelCount != 0 && st.Difficulty.LevelCount < 10 {
		return fmt.Errorf("strategy.difficulty.levelCount: min 10, got %d", st.Difficulty.LevelCount)
	}
	wl := r.WinLose
	if wl.Win.Condition == "" {
		return fmt.Errorf("winLose.win.condition: required")
	}
	if err := oneOf("winLose.win.type", wl.Win.Type,
		"score_attack", "level_clear", "endless_highscore", "time_attack", "collection"); err != nil {
		return err
	}
	if err := oneOf("winLose.win.grading", wl.Win.Grading, "stars_3", "letter_grades", "none"); err != nil {
		return err
	}
	if wl.Lose.Condition == "" {
		return fmt.Errorf("winLose.lose.condition: required")
	}
	if err := oneOf("winLose.failHandling.deathKind", wl.FailHandling.DeathKind,
		"time_penalty", "instant_restart", "checkpoint_respawn"); err != nil {
		return err
	}
	if err := oneOf("winLose.progression", wl.Progression, "none", "levels", "endless_meta", "tournament"); err != nil {
		return err
	}
	h := r.Hook
	if h.TimeToInputSec > 5 {
		return fmt.Errorf("hook.timeToInputSec: max 5, got %v", h.TimeToInputSec)
	}
	if h.TimeToFirstRewardSec > 15 {
		return fmt.Errorf("hook.timeToFirstRewardSec: max 15, got %v", h.TimeToFirstRewardSec)
	}
	if err := oneOf("hook.firstRewardKind", h.FirstRewardKind, "pickup", "score_tick", "capture", "clear", "unlock"); err != nil {
		return err
	}
	if err := oneOf("hook.tutorial.kind", h.Tutorial.Kind, "none", "learn_by_doing", "overlay_hints"); err != nil {
		return err
	}
	if h.Restart.MaxSec > 2 {
		return fmt.Errorf("hook.restart.maxSec: max 2, got %v", h.Restart.MaxSec)
	}
	if h.Restart.MaxClicks > 1 {
		return fmt.Errorf("hook.restart.maxClicks: max 1, got %d", h.Restart.MaxClicks)
	}
	if err := oneOf("audio.music.mood", r.Audio.Music.Mood, "cheerful", "tense", "calm", "epic", "silly", "mysterious"); err != nil {
		return err
	}
	if err := oneOf("audio.music.style", r.Audio.Music.Style, "chiptune", "electronic", "orchestral", "acoustic", "none"); err != nil {
		return err
	}
	if err := oneOf("visual.artStyle", r.Visual.ArtStyle, "kid_drawing_preserved", "pixel", "flat_vector", "cartoon"); err != nil {
		return err
	}
	if r.Visual.ArtPolicy != "" && r.Visual.ArtPolicy != ArtPreserve {
		return fmt.Errorf("visual.artPolicy: must be %q, got %q", ArtPreserve, r.Visual.ArtPolicy)
	}
	if err := oneOf("visual.background.kind", r.Visual.Background.Kind,
		"static_image", "video_loop", "procedural", "parallax_layers"); err != nil {
		return err
	}
	if err := oneOf("visual.effectsBudget", r.Visual.EffectsBudget, "none", "minimal", "rich"); err != nil {
		return err
	}
	for i, c := range r.Visual.Palette {
		if !hexColor.MatchString(c) {
			return fmt.Errorf("visual.palette[%d]: want #RRGGBB, got %q", i, c)
		}
	}
	if r.Characters.Player.Name == "" {
		return fmt.Errorf("characters.player.name: required")
	}
	if r.Characters.Player.DesignRef == "" {
		return fmt.Errorf("characters.player.designRef: required")
	}
	for i, n := range r.Characters.NPCs {
		if n.Name == "" {
			return fmt.Errorf("characters.npcs[%d].name: required", i)
		}
		if err := reqOneOf(fmt.Sprintf("characters.npcs[%d].role", i), n.Role, "enemy", "friend", "neutral", "collectible"); err != nil {
			return err
		}
		if n.DesignRef == "" {
			return fmt.Errorf("characters.npcs[%d].designRef: required", i)
		}
	}
	if err := oneOf("features.coop.inputScheme", r.Features.Coop.InputScheme,
		"shared_keyboard_arrows_wasd", "shared_touch", "none"); err != nil {
		return err
	}
	// v1.1: mechanic diet (Li Dan) — every declared mechanic justifies itself
	// with a loopLink, or it gets cut.
	for i, p := range r.Features.Powerups {
		if p.Name == "" {
			return fmt.Errorf("features.powerups[%d].name: required", i)
		}
		if p.LoopLink == "" {
			return fmt.Errorf("features.powerups[%d].loopLink: required — every mechanic must state how it serves the core loop", i)
		}
	}
	for i, m := range r.Features.Modes {
		if m.Name == "" {
			return fmt.Errorf("features.modes[%d].name: required", i)
		}
		if m.LoopLink == "" {
			return fmt.Errorf("features.modes[%d].loopLink: required — every mechanic must state how it serves the core loop", i)
		}
	}
	c := r.Constraints
	if c.SessionTargetSec > 90 {
		return fmt.Errorf("constraints.sessionTargetSec: max 90, got %d", c.SessionTargetSec)
	}
	if c.PackageMB > 25 {
		return fmt.Errorf("constraints.packageMB: max 25, got %v", c.PackageMB)
	}
	if c.ColdStartSec > 3 {
		return fmt.Errorf("constraints.coldStartSec: max 3, got %v", c.ColdStartSec)
	}
	if err := oneOf("constraints.audience", c.Audience, "kids", "family", "general"); err != nil {
		return err
	}
	if err := oneOf("constraints.perfBudget", c.PerfBudget, "low_end", "mid", "high"); err != nil {
		return err
	}
	for k, v := range r.Provenance {
		if err := reqOneOf("provenance["+k+"]", v, "upload", "inferred", "user_stated", "default"); err != nil {
			return err
		}
	}
	return nil
}
