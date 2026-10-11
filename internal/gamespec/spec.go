// Package gamespec standardizes game generation into factors.
//
// Every game is reduced to a GameSpec: a fixed set of factors covering
// identity, source material, gameplay, visuals, audio, systems, and platform.
// The spec is filled from the user's preferences plus the questionnaire
// (questionnaire.go), then gamespec.Decide routes it to the right engine
// (decide.go): HTML5 canvas for simple games, Godot for effect-heavy ones.
package gamespec

import (
	"fmt"
	"strings"
)

// Engines.
const (
	EngineHTML5 = "html5"
	EngineGodot = "godot"
)

// SpecVersion is the current GameSpec schema version.
const SpecVersion = 1

// Enumerated factor values.
const (
	AudienceKids35  = "kids-3-5"
	AudienceKids47  = "kids-4-7"
	AudienceKids812 = "kids-8-12"
	AudienceFamily  = "family"
	AudienceTeens   = "teens"
	AudienceAdults  = "adults"

	SourceDrawing = "kids-drawing"
	SourcePrompt  = "text-prompt"
	SourceRemix   = "remix"

	ArtPreserve  = "preserve"
	ArtPolish    = "polish"
	ArtReimagine = "reimagine"

	ArchCatcher   = "catcher"
	ArchDodger    = "dodger"
	ArchTapper    = "tapper"
	ArchRunner    = "runner"
	ArchPlatform  = "platformer"
	ArchPuzzle    = "puzzle"
	ArchShooter   = "shooter"
	ArchRacer     = "racer"
	ArchStory     = "story"
	ArchSandbox   = "sandbox"

	WinScoreTarget  = "score-target"
	WinSurvivalTime = "survival-time"
	WinCompletion   = "completion"
	WinEndless      = "endless"

	LoseLives  = "lives"
	LoseTimer  = "timer"
	LoseCaught = "caught"
	LoseNone   = "none"

	InputTap        = "tap"
	InputDrag       = "drag"
	InputSwipe      = "swipe"
	InputTilt       = "tilt"
	InputKeys       = "keys"
	InputMultitouch = "multitouch"

	StyleKidDrawing = "kid-drawing"
	StyleCartoon    = "cartoon"
	StylePixel      = "pixel"
	StyleVectorFlat = "vector-flat"
	StyleRealistic3D = "realistic-3d"

	ParticlesNone   = "none"
	ParticlesSimple = "simple"
	ParticlesRich   = "rich"

	AnimNone     = "none"
	AnimTween    = "tween"
	AnimSkeletal = "skeletal"
	AnimFrame    = "frame"

	MusicNone      = "none"
	MusicProcLoop  = "procedural-loop"
	MusicLicensed  = "licensed"

	MoodCheerful = "cheerful"
	MoodCalm     = "calm"
	MoodTense    = "tense"
	MoodEpic     = "epic"

	ProgNone        = "none"
	ProgLevels      = "levels"
	ProgEndlessMeta = "endless-meta"

	DiffEasy   = "easy"
	DiffNormal = "normal"
	DiffHard   = "hard"

	TargetWeb     = "web"
	TargetIOS     = "ios"
	TargetAndroid = "android"
	TargetDesktop = "desktop"

	OrientPortrait  = "portrait"
	OrientLandscape = "landscape"

	PerfInstant = "instant"
	PerfLowEnd  = "low-end"
	PerfMid     = "mid"
	PerfHigh    = "high"
)

// SourceElement is one recognized element of the source material
// (e.g. from the vision ingest of a kid's drawing).
type SourceElement struct {
	Name string `json:"name"`
	Role string `json:"role"` // player | obstacle | pickup | background | decor
}

// WinCondition describes how a run is won.
type WinCondition struct {
	Type  string `json:"type"`
	Value int    `json:"value,omitempty"`
}

// GameSpec is the standardized factor set for one game.
type GameSpec struct {
	SpecVersion int `json:"specVersion"`

	// Identity.
	Title            string `json:"title"`
	Logline          string `json:"logline,omitempty"`
	Audience         string `json:"audience,omitempty"`
	SessionTargetSec int    `json:"sessionTargetSec,omitempty"`

	// Source.
	SourceKind  string          `json:"sourceKind,omitempty"`
	SourceElems []SourceElement `json:"sourceElements,omitempty"`
	ArtPolicy   string          `json:"artPolicy,omitempty"`

	// Gameplay.
	Archetype string       `json:"archetype"`
	CoreLoop  string       `json:"coreLoop"`
	Win       WinCondition `json:"win"`
	Lose      string       `json:"lose,omitempty"`
	Input     []string     `json:"input"`
	Players   int          `json:"players,omitempty"`

	// Visual.
	Style          string `json:"style,omitempty"`
	SpriteCount    int    `json:"spriteCount,omitempty"`
	Particles      string `json:"particles,omitempty"`
	Lighting       bool   `json:"lighting,omitempty"`
	ParallaxLayers int    `json:"parallaxLayers,omitempty"`
	ScreenShake    bool   `json:"screenShake,omitempty"`
	PostFx         bool   `json:"postFx,omitempty"`
	ThreeD         bool   `json:"threeD,omitempty"`
	Animation      string `json:"animation,omitempty"`

	// Audio.
	MusicKind string   `json:"musicKind,omitempty"`
	MusicMood string   `json:"musicMood,omitempty"`
	Sfx       []string `json:"sfx,omitempty"`

	// Systems.
	Progression     string   `json:"progression,omitempty"`
	DifficultyRamp  float64  `json:"difficultyRamp,omitempty"`
	Powerups        []string `json:"powerups,omitempty"`
	Combo           bool     `json:"combo,omitempty"`
	Leaderboard     bool     `json:"leaderboard,omitempty"`
	Difficulty      string   `json:"difficulty,omitempty"`

	// Platform.
	Targets      []string `json:"targets,omitempty"`
	Orientation  string   `json:"orientation,omitempty"`
	PerfBudget   string   `json:"perfBudget,omitempty"`
}

// UserPrefs are per-user defaults overlaid onto a spec. The questionnaire
// only asks about these when no saved value exists.
type UserPrefs struct {
	Difficulty      string `json:"difficulty,omitempty"`
	SoundOn         bool   `json:"soundOn"`
	ArtPolicy       string `json:"artPolicy,omitempty"`
	SessionTargetSec int   `json:"sessionTargetSec,omitempty"`
	PerfBudget      string `json:"perfBudget,omitempty"`
	Audience        string `json:"audience,omitempty"`
}

// DefaultPrefs returns the standing defaults (established from shipped games).
func DefaultPrefs() UserPrefs {
	return UserPrefs{
		Difficulty:       DiffEasy,
		SoundOn:          true,
		ArtPolicy:        ArtPreserve,
		SessionTargetSec: 90,
		PerfBudget:       PerfLowEnd,
		Audience:         AudienceFamily,
	}
}

// ApplyDefaults fills zero-valued factors from defaults and user prefs.
// Call this after the assistant/user fills the known factors.
func (s *GameSpec) ApplyDefaults(prefs UserPrefs) {
	if s.SpecVersion == 0 {
		s.SpecVersion = SpecVersion
	}
	if prefs == (UserPrefs{}) {
		prefs = DefaultPrefs()
	}
	set := func(dst *string, v, fb string) {
		if *dst == "" {
			*dst = v
			if *dst == "" {
				*dst = fb
			}
		}
	}
	set(&s.Audience, prefs.Audience, AudienceFamily)
	if s.SessionTargetSec == 0 {
		s.SessionTargetSec = prefs.SessionTargetSec
		if s.SessionTargetSec == 0 {
			s.SessionTargetSec = 90
		}
	}
	set(&s.SourceKind, "", SourceDrawing)
	set(&s.ArtPolicy, prefs.ArtPolicy, ArtPreserve)
	if s.Win.Type == "" {
		s.Win = WinCondition{Type: WinEndless}
	}
	set(&s.Lose, "", LoseLives)
	if s.Players == 0 {
		s.Players = 1
	}
	set(&s.Style, "", StyleKidDrawing)
	if s.SpriteCount == 0 {
		s.SpriteCount = 12
	}
	set(&s.Particles, "", ParticlesNone)
	if s.ParallaxLayers == 0 {
		s.ParallaxLayers = 1
	}
	set(&s.Animation, "", AnimTween)
	if s.MusicKind == "" {
		if prefs.SoundOn {
			s.MusicKind = MusicProcLoop
		} else {
			s.MusicKind = MusicNone
		}
	}
	set(&s.MusicMood, "", MoodCheerful)
	set(&s.Progression, "", ProgEndlessMeta)
	if s.DifficultyRamp == 0 {
		s.DifficultyRamp = 1.25
	}
	if !s.Combo && !s.Leaderboard {
		s.Leaderboard = true
	}
	set(&s.Difficulty, prefs.Difficulty, DiffEasy)
	if len(s.Targets) == 0 {
		s.Targets = []string{TargetWeb}
	}
	set(&s.Orientation, "", OrientPortrait)
	set(&s.PerfBudget, prefs.PerfBudget, PerfLowEnd)
}

// Validate checks required factors and enum values.
func (s *GameSpec) Validate() error {
	var errs []string
	req := func(name, v string) {
		if strings.TrimSpace(v) == "" {
			errs = append(errs, name+" is required")
		}
	}
	req("title", s.Title)
	req("archetype", s.Archetype)
	req("coreLoop", s.CoreLoop)
	if len(s.Input) == 0 {
		errs = append(errs, "input is required (at least one)")
	}
	oneOf := func(name, v string, opts ...string) {
		if v == "" {
			return
		}
		for _, o := range opts {
			if v == o {
				return
			}
		}
		errs = append(errs, fmt.Sprintf("%s has invalid value %q", name, v))
	}
	oneOf("audience", s.Audience, AudienceKids35, AudienceKids47, AudienceKids812, AudienceFamily, AudienceTeens, AudienceAdults)
	oneOf("sourceKind", s.SourceKind, SourceDrawing, SourcePrompt, SourceRemix)
	oneOf("artPolicy", s.ArtPolicy, ArtPreserve, ArtPolish, ArtReimagine)
	oneOf("archetype", s.Archetype, ArchCatcher, ArchDodger, ArchTapper, ArchRunner, ArchPlatform, ArchPuzzle, ArchShooter, ArchRacer, ArchStory, ArchSandbox)
	oneOf("win.type", s.Win.Type, WinScoreTarget, WinSurvivalTime, WinCompletion, WinEndless)
	oneOf("lose", s.Lose, LoseLives, LoseTimer, LoseCaught, LoseNone)
	for _, in := range s.Input {
		oneOf("input[]", in, InputTap, InputDrag, InputSwipe, InputTilt, InputKeys, InputMultitouch)
	}
	oneOf("style", s.Style, StyleKidDrawing, StyleCartoon, StylePixel, StyleVectorFlat, StyleRealistic3D)
	oneOf("particles", s.Particles, ParticlesNone, ParticlesSimple, ParticlesRich)
	oneOf("animation", s.Animation, AnimNone, AnimTween, AnimSkeletal, AnimFrame)
	oneOf("musicKind", s.MusicKind, MusicNone, MusicProcLoop, MusicLicensed)
	oneOf("musicMood", s.MusicMood, MoodCheerful, MoodCalm, MoodTense, MoodEpic)
	oneOf("progression", s.Progression, ProgNone, ProgLevels, ProgEndlessMeta)
	oneOf("difficulty", s.Difficulty, DiffEasy, DiffNormal, DiffHard)
	for _, t := range s.Targets {
		oneOf("targets[]", t, TargetWeb, TargetIOS, TargetAndroid, TargetDesktop)
	}
	oneOf("orientation", s.Orientation, OrientPortrait, OrientLandscape)
	oneOf("perfBudget", s.PerfBudget, PerfInstant, PerfLowEnd, PerfMid, PerfHigh)
	if s.SpriteCount < 0 {
		errs = append(errs, "spriteCount cannot be negative")
	}
	if s.DifficultyRamp < 1 {
		errs = append(errs, "difficultyRamp must be >= 1")
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalid spec: %s", strings.Join(errs, "; "))
	}
	return nil
}
