package gamespec

import "testing"

func boboSpec() GameSpec {
	return GameSpec{
		Title:    "Bobo's Rainbow",
		Logline:  "Move Bobo to catch falling hearts and complete the rainbow.",
		Archetype: ArchCatcher,
		CoreLoop: "move the basket, catch falling hearts",
		Win:      WinCondition{Type: WinScoreTarget, Value: 12},
		Lose:     LoseLives,
		Input:    []string{InputTap, InputDrag},
		SourceElems: []SourceElement{
			{Name: "bear", Role: "player"},
			{Name: "kitten", Role: "decor"},
			{Name: "heart", Role: "pickup"},
			{Name: "rainbow", Role: "background"},
		},
		SpriteCount: 12,
		Particles:   ParticlesNone,
		Powerups:    []string{"golden-heart"},
		Combo:       true,
	}
}

func skySpec() GameSpec {
	return GameSpec{
		Title:     "Ribbon Mends the Sky",
		Logline:   "Tap holes in the sky to stitch them before demons get you.",
		Archetype: ArchTapper,
		CoreLoop:  "tap sky holes to stitch them, dodge demons",
		Win:       WinCondition{Type: WinScoreTarget, Value: 6},
		Lose:      LoseLives,
		Input:     []string{InputTap},
		SpriteCount: 10,
		Particles:   ParticlesNone,
		Powerups:    []string{"lucky-shard"},
	}
}

func escapeSpec() GameSpec {
	return GameSpec{
		Title:     "Escape from the Injection",
		Logline:   "Dodge syringes across three lanes and outrun the chaser.",
		Archetype: ArchDodger,
		CoreLoop:  "switch lanes to dodge syringes and grab courage stars",
		Win:       WinCondition{Type: WinEndless},
		Lose:      LoseCaught,
		Input:     []string{InputTap, InputSwipe},
		SpriteCount:    15,
		Particles:      ParticlesNone,
		ParallaxLayers: 2,
		ScreenShake:    true,
	}
}

// Since 2026-10-20 there is one runtime (PixiJS + Rapier); every spec,
// simple or complex, decides to it. These tests pin that policy across
// the variety that used to exercise the html5/godot router.
func TestDecideAlwaysPixi(t *testing.T) {
	threeD := boboSpec()
	threeD.ThreeD = true
	threeD.Style = StyleRealistic3D

	instant := boboSpec()
	instant.PerfBudget = PerfInstant

	fireworks := GameSpec{
		Title:     "Fireworks Sandbox",
		Archetype: ArchSandbox,
		CoreLoop:  "launch fireworks, paint the sky",
		Input:     []string{InputTap, InputMultitouch},
		Particles: ParticlesRich,
		Lighting:  true,
		PostFx:    true,
		Animation: AnimSkeletal,
		SpriteCount: 120,
		PerfBudget:  PerfHigh,
	}

	simple := GameSpec{
		Title:     "Tap the Apple",
		Archetype: ArchTapper,
		CoreLoop:  "tap the falling apples",
		Input:     []string{InputTap},
		SpriteCount: 8,
		Particles:   ParticlesNone,
		PerfBudget:  PerfLowEnd,
	}

	for name, spec := range map[string]GameSpec{
		"bobos-rainbow": boboSpec(),
		"ribbon-sky":    skySpec(),
		"escape":        escapeSpec(),
		"three-d":       threeD,
		"instant-load":  instant,
		"fireworks":     fireworks,
		"simple":        simple,
	} {
		d := Decide(spec)
		if d.Engine != EnginePixi {
			t.Errorf("%s: engine=%s, want pixi (rationale: %v)", name, d.Engine, d.Rationale)
		}
		if len(d.Rationale) == 0 {
			t.Errorf("%s: empty rationale", name)
		}
		if d.Scores[EnginePixi] != 1000 {
			t.Errorf("%s: scores=%v, want pixi:1000", name, d.Scores)
		}
	}
}

func TestDecideDeterministic(t *testing.T) {
	a, b := Decide(boboSpec()), Decide(boboSpec())
	if a.Engine != b.Engine || a.Scores[EnginePixi] != b.Scores[EnginePixi] {
		t.Errorf("decide is not deterministic: %+v vs %+v", a, b)
	}
}

func TestValidate(t *testing.T) {
	var s GameSpec
	if err := s.Validate(); err == nil {
		t.Error("empty spec should fail validation")
	}
	bad := boboSpec()
	bad.Archetype = "nope"
	if err := bad.Validate(); err == nil {
		t.Error("bad archetype should fail validation")
	}
	neg := boboSpec()
	neg.SpriteCount = -1
	if err := neg.Validate(); err == nil {
		t.Error("negative spriteCount should fail validation")
	}
}

func TestQuestionnaireCoversSpec(t *testing.T) {
	q := Questionnaire()
	if len(q) < 25 {
		t.Errorf("questionnaire has %d factors, want >= 25", len(q))
	}
	seen := map[string]bool{}
	for _, f := range q {
		if seen[f.Key] {
			t.Errorf("duplicate factor %q", f.Key)
		}
		seen[f.Key] = true
	}
	for _, k := range []string{"title", "archetype", "coreLoop", "input", "difficulty", "perfBudget", "artPolicy"} {
		if !seen[k] {
			t.Errorf("questionnaire missing factor %q", k)
		}
	}
	prefs := 0
	for _, f := range q {
		if f.Preference {
			prefs++
		}
	}
	if prefs < 4 {
		t.Errorf("only %d preference factors, want >= 4", prefs)
	}
}

func TestApplyDefaults(t *testing.T) {
	s := GameSpec{Title: "x", Archetype: ArchTapper, CoreLoop: "tap things", Input: []string{InputTap}}
	s.ApplyDefaults(DefaultPrefs())
	if s.ArtPolicy != ArtPreserve {
		t.Errorf("artPolicy=%s, want preserve", s.ArtPolicy)
	}
	if s.Progression != ProgEndlessMeta {
		t.Errorf("progression=%s, want endless-meta", s.Progression)
	}
	if !s.Leaderboard {
		t.Error("leaderboard should default true")
	}
	if s.DifficultyRamp != 1.25 {
		t.Errorf("ramp=%v, want 1.25", s.DifficultyRamp)
	}
}
