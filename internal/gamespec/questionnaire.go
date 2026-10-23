package gamespec

// Factor describes one questionnaire entry: the prompt shown to fill a
// factor, its type, allowed options, default, and whether it is a standing
// user preference (asked once, remembered) rather than a per-game question.
type Factor struct {
	Key        string   `json:"key"`
	Group      string   `json:"group"`
	Prompt     string   `json:"prompt"`
	Type       string   `json:"type"` // enum | enum-list | int | float | bool | string | string-list
	Options    []string `json:"options,omitempty"`
	Default    string   `json:"default,omitempty"`
	Preference bool     `json:"preference"`
	Required   bool     `json:"required"`
}

// Questionnaire is the full ordered factor list. A creator UI (or the iPhone
// app) can render the detailed questionnaire directly from this.
func Questionnaire() []Factor {
	return []Factor{
		// Identity.
		{Key: "title", Group: "identity", Prompt: "What is the game called?", Type: "string", Required: true},
		{Key: "logline", Group: "identity", Prompt: "One-sentence pitch: what do you do?", Type: "string"},
		{Key: "audience", Group: "identity", Prompt: "Who is this game for?", Type: "enum",
			Options: []string{AudienceKids35, AudienceKids47, AudienceKids812, AudienceFamily, AudienceTeens, AudienceAdults},
			Default: AudienceFamily, Preference: true},
		{Key: "sessionTargetSec", Group: "identity", Prompt: "How long should one sitting last (seconds)?", Type: "int",
			Default: "90", Preference: true},

		// Source.
		{Key: "sourceKind", Group: "source", Prompt: "Where does the game come from?", Type: "enum",
			Options: []string{SourceDrawing, SourcePrompt, SourceRemix}, Default: SourceDrawing},
		{Key: "artPolicy", Group: "source", Prompt: "How should the kid's art be treated?", Type: "enum",
			Options: []string{ArtPreserve, ArtPolish, ArtReimagine}, Default: ArtPreserve, Preference: true},

		// Gameplay.
		{Key: "archetype", Group: "gameplay", Prompt: "What kind of game is it?", Type: "enum",
			Options:  []string{ArchCatcher, ArchDodger, ArchTapper, ArchRunner, ArchPlatform, ArchPuzzle, ArchShooter, ArchRacer, ArchStory, ArchSandbox},
			Required: true},
		{Key: "coreLoop", Group: "gameplay", Prompt: "Core loop in a few words (verb the object)?", Type: "string", Required: true},
		{Key: "win.type", Group: "gameplay", Prompt: "How do you win?", Type: "enum",
			Options: []string{WinScoreTarget, WinSurvivalTime, WinCompletion, WinEndless}, Default: WinEndless},
		{Key: "lose", Group: "gameplay", Prompt: "How do you lose?", Type: "enum",
			Options: []string{LoseLives, LoseTimer, LoseCaught, LoseNone}, Default: LoseLives},
		{Key: "input", Group: "gameplay", Prompt: "How do you control it?", Type: "enum-list",
			Options: []string{InputTap, InputDrag, InputSwipe, InputTilt, InputKeys, InputMultitouch}, Required: true},

		// Visual.
		{Key: "style", Group: "visual", Prompt: "Art style?", Type: "enum",
			Options: []string{StyleKidDrawing, StyleCartoon, StylePixel, StyleVectorFlat, StyleRealistic3D}, Default: StyleKidDrawing},
		{Key: "spriteCount", Group: "visual", Prompt: "Roughly how many sprites on screen at once?", Type: "int", Default: "12"},
		{Key: "particles", Group: "visual", Prompt: "Particle effects?", Type: "enum",
			Options: []string{ParticlesNone, ParticlesSimple, ParticlesRich}, Default: ParticlesNone},
		{Key: "lighting", Group: "visual", Prompt: "Dynamic lighting / shadows?", Type: "bool", Default: "false"},
		{Key: "parallaxLayers", Group: "visual", Prompt: "How many parallax background layers?", Type: "int", Default: "1"},
		{Key: "screenShake", Group: "visual", Prompt: "Screen shake on impacts?", Type: "bool", Default: "false"},
		{Key: "postFx", Group: "visual", Prompt: "Post-processing (glow, blur, vignette)?", Type: "bool", Default: "false"},
		{Key: "threeD", Group: "visual", Prompt: "Real 3D rendering?", Type: "bool", Default: "false"},
		{Key: "animation", Group: "visual", Prompt: "How do characters animate?", Type: "enum",
			Options: []string{AnimNone, AnimTween, AnimSkeletal, AnimFrame}, Default: AnimTween},

		// Audio.
		{Key: "musicMood", Group: "audio", Prompt: "Music mood?", Type: "enum",
			Options: []string{MoodCheerful, MoodCalm, MoodTense, MoodEpic}, Default: MoodCheerful},
		{Key: "sfx", Group: "audio", Prompt: "Sound effects needed (comma-separated)?", Type: "string-list"},

		// Systems.
		{Key: "progression", Group: "systems", Prompt: "Progression structure?", Type: "enum",
			Options: []string{ProgNone, ProgLevels, ProgEndlessMeta}, Default: ProgEndlessMeta},
		{Key: "difficultyRamp", Group: "systems", Prompt: "Speed multiplier per level (e.g. 1.25)?", Type: "float", Default: "1.25"},
		{Key: "powerups", Group: "systems", Prompt: "Power-ups (comma-separated)?", Type: "string-list"},
		{Key: "combo", Group: "systems", Prompt: "Combo / multiplier scoring?", Type: "bool", Default: "false"},
		{Key: "leaderboard", Group: "systems", Prompt: "Leaderboard?", Type: "bool", Default: "true"},
		{Key: "difficulty", Group: "systems", Prompt: "Default difficulty?", Type: "enum",
			Options: []string{DiffEasy, DiffNormal, DiffHard}, Default: DiffEasy, Preference: true},

		// Platform.
		{Key: "targets", Group: "platform", Prompt: "Where should it run?", Type: "enum-list",
			Options: []string{TargetWeb, TargetIOS, TargetAndroid, TargetDesktop}, Default: TargetWeb},
		{Key: "orientation", Group: "platform", Prompt: "Orientation?", Type: "enum",
			Options: []string{OrientPortrait, OrientLandscape}, Default: OrientPortrait},
		{Key: "perfBudget", Group: "platform", Prompt: "Performance budget?", Type: "enum",
			Options: []string{PerfInstant, PerfLowEnd, PerfMid, PerfHigh}, Default: PerfLowEnd, Preference: true},
	}
}
