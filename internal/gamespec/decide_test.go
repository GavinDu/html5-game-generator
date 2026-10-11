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

func TestDecideShippedGamesAreHTML5(t *testing.T) {
	for name, spec := range map[string]GameSpec{
		"bobos-rainbow": boboSpec(),
		"ribbon-sky":    skySpec(),
		"escape":        escapeSpec(),
	} {
		spec.ApplyDefaults(DefaultPrefs())
		if err := spec.Validate(); err != nil {
			t.Fatalf("%s: validate: %v", name, err)
		}
		d := Decide(spec)
		if d.Engine != EngineHTML5 {
			t.Errorf("%s: engine=%s, want html5 (rationale: %v)", name, d.Engine, d.Rationale)
		}
		if len(d.Rationale) == 0 {
			t.Errorf("%s: empty rationale", name)
		}
	}
}

func TestDecideHardRules(t *testing.T) {
	threeD := boboSpec()
	threeD.ThreeD = true
	threeD.Style = StyleRealistic3D
	if d := Decide(threeD); d.Engine != EngineGodot {
		t.Errorf("3D spec: engine=%s, want godot", d.Engine)
	}

	instant := boboSpec()
	instant.PerfBudget = PerfInstant
	if d := Decide(instant); d.Engine != EngineHTML5 {
		t.Errorf("instant-load spec: engine=%s, want html5", d.Engine)
	}
}

func TestDecideComplexGoesGodot(t *testing.T) {
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
	if d := Decide(fireworks); d.Engine != EngineGodot {
		t.Errorf("fireworks: engine=%s, want godot (scores %v)", d.Engine, d.Scores)
	}

	platformer := GameSpec{
		Title:          "Shadow Knight",
		Archetype:      ArchPlatform,
		CoreLoop:       "run, jump, slash through torch-lit caves",
		Input:          []string{InputSwipe, InputTap},
		Lighting:       true,
		ParallaxLayers: 4,
		ScreenShake:    true,
		Particles:      ParticlesSimple,
		SpriteCount:    40,
		PerfBudget:     PerfMid,
	}
	if d := Decide(platformer); d.Engine != EngineGodot {
		t.Errorf("platformer: engine=%s, want godot (scores %v)", d.Engine, d.Scores)
	}
}

func TestDecideLeansGodot(t *testing.T) {
	// Mid-complexity: simple archetype, but enough juice that the old
	// HTML5-default would have kept it on canvas. Now it goes Godot.
	mid := GameSpec{
		Title:     "Star Dodger",
		Archetype: ArchDodger,
		CoreLoop:  "dodge comets, grab stars",
		Input:     []string{InputTap, InputDrag},
		SpriteCount: 25,
		Particles:   ParticlesSimple,
		ScreenShake: true,
		PerfBudget:  PerfMid,
	}
	mid.ApplyDefaults(DefaultPrefs())
	if err := mid.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if d := Decide(mid); d.Engine != EngineGodot {
		t.Errorf("mid-complexity: engine=%s, want godot (scores %v, rationale %v)",
			d.Engine, d.Scores, d.Rationale)
	}

	// Decisively simple still goes HTML5: the instant-load feed game.
	simple := GameSpec{
		Title:     "Tap the Apple",
		Archetype: ArchTapper,
		CoreLoop:  "tap the falling apples",
		Input:     []string{InputTap},
		SpriteCount: 8,
		Particles:   ParticlesNone,
		PerfBudget:  PerfLowEnd,
	}
	simple.ApplyDefaults(DefaultPrefs())
	if d := Decide(simple); d.Engine != EngineHTML5 {
		t.Errorf("simple game: engine=%s, want html5 (scores %v)", d.Engine, d.Scores)
	}
}

func TestDecideDeterministic(t *testing.T) {
	a, b := Decide(boboSpec()), Decide(boboSpec())
	if a.Engine != b.Engine || a.Scores[EngineHTML5] != b.Scores[EngineHTML5] {
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
