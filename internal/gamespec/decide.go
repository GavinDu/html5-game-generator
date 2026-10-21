package gamespec

// Decision is the engine verdict for a GameSpec. Since the 2026-10-20
// single-runtime decision there is exactly one engine — PixiJS + Rapier —
// so the verdict is constant; the type is kept so API consumers and the
// questionnaire flow keep their shape.
type Decision struct {
	Engine    string         `json:"engine"`    // always EnginePixi
	Rationale []string       `json:"rationale"` // why this is the (only) runtime
	Scores    map[string]int `json:"scores"`    // {"pixi": 1000}
}

// Decide returns the single-runtime verdict for a spec. Pure and
// deterministic. The spec's factors still matter — they shape generation,
// the questionnaire, and the IR the game is authored into — but they no
// longer choose an engine: PixiJS (rendering) + Rapier (physics) is the
// runtime for every game, chosen because physics simulation is a core
// expressive medium for user-created interactive content, not only a
// playback concern.
//
// History: until 2026-10-20 this routed between HTML5 canvas and Godot
// with hard rules and weighted scores (Godot-leaning since 2026-10-10).
// That routing, the Godot web-export endpoints, the Defold reference
// runtime, and the native Swift IR interpreter were all removed in favor
// of the single runtime; Game IR remains the cross-platform keyframe
// contract every game is authored into.
func Decide(spec GameSpec) Decision {
	spec.ApplyDefaults(DefaultPrefs())
	return Decision{
		Engine: EnginePixi,
		Rationale: []string{
			"Single runtime: PixiJS + Rapier interprets the game's Game IR package.",
		},
		Scores: map[string]int{EnginePixi: 1000},
	}
}
