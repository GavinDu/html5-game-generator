package gamespec

import (
	"fmt"
	"sort"
)

// Decision is the engine routing verdict for a GameSpec.
type Decision struct {
	Engine    string         `json:"engine"`    // EngineHTML5 or EngineGodot
	Rationale []string       `json:"rationale"` // top contributing factors, plain language
	Scores    map[string]int `json:"scores"`    // {"html5": n, "godot": m}
}

// scoreStep is one weighted signal with its human explanation.
type scoreStep struct {
	engine string
	points int
	why    string
}

// Decide routes a spec to an engine. Pure and deterministic: the same spec
// always yields the same engine, so the router is testable and tunable
// without touching the generators.
//
// Policy (2026-10-10): Godot is the default. It is the better game engine
// (real physics, particles, lighting, juice, headless validation), so any
// game with a hint of richness goes there. HTML5 is reserved for decisively
// simple games where instant loading matters more than engine power —
// typically short-session, low-end-budget feed games.
func Decide(spec GameSpec) Decision {
	spec.ApplyDefaults(DefaultPrefs())
	var steps []scoreStep
	add := func(engine string, points int, why string) {
		if points != 0 {
			steps = append(steps, scoreStep{engine, points, why})
		}
	}

	// Hard rules first.
	if spec.ThreeD {
		return Decision{
			Engine:    EngineGodot,
			Rationale: []string{"3D rendering requires the Godot engine."},
			Scores:    map[string]int{EngineHTML5: 0, EngineGodot: 1000},
		}
	}
	if spec.PerfBudget == PerfInstant {
		return Decision{
			Engine: EngineHTML5,
			Rationale: []string{
				"Must be interactive in ~2 seconds on slow networks; the Godot WASM payload (~40MB) disqualifies it.",
			},
			Scores: map[string]int{EngineHTML5: 1000, EngineGodot: 0},
		}
	}

	// Weighted signals: Godot.
	if spec.Particles == ParticlesRich {
		add(EngineGodot, 30, "rich particle effects play to Godot's CPUParticles2D")
	}
	if spec.Animation == AnimSkeletal {
		add(EngineGodot, 25, "skeletal animation wants Godot's rigging")
	}
	if spec.Lighting {
		add(EngineGodot, 25, "dynamic lighting/shadows need Godot's renderers")
	}
	if spec.PostFx {
		add(EngineGodot, 20, "post-processing effects are native in Godot")
	}
	if spec.ParallaxLayers >= 3 {
		add(EngineGodot, 15, fmt.Sprintf("%d parallax layers compose easier as Godot Parallax2D", spec.ParallaxLayers))
	}
	if spec.ScreenShake {
		add(EngineGodot, 10, "camera shake/juice is cheap in Godot")
	}
	if spec.SpriteCount > 60 {
		add(EngineGodot, 20, fmt.Sprintf("~%d simultaneous sprites favor Godot's scene tree", spec.SpriteCount))
	}
	switch spec.Archetype {
	case ArchPlatform, ArchShooter, ArchRacer, ArchSandbox:
		add(EngineGodot, 15, spec.Archetype+" mechanics lean on physics and scene composition")
	}

	// Weighted signals: HTML5.
	if spec.PerfBudget == PerfLowEnd {
		add(EngineHTML5, 20, "low-end mobile budget favors instant-load HTML5 over a 40MB WASM payload")
	}
	switch spec.Archetype {
	case ArchCatcher, ArchTapper, ArchDodger, ArchRunner, ArchPuzzle, ArchStory:
		add(EngineHTML5, 15, spec.Archetype+" is simple enough that canvas 2D wins on ease and performance")
	}
	if spec.SpriteCount <= 15 {
		add(EngineHTML5, 10, fmt.Sprintf("only ~%d sprites: no need for an engine scene tree", spec.SpriteCount))
	}
	if spec.Particles == ParticlesNone {
		add(EngineHTML5, 10, "no particles to simulate")
	}
	if (spec.Animation == AnimNone || spec.Animation == AnimTween) &&
		!spec.Lighting && !spec.PostFx && !spec.ScreenShake && spec.Particles == ParticlesNone {
		add(EngineHTML5, 10, "all-simple rendering: tweens and canvas compositing are enough")
	}
	if spec.SessionTargetSec <= 120 {
		add(EngineHTML5, 5, "short session: HTML5's zero-load start matters")
	}

	scores := map[string]int{EngineHTML5: 0, EngineGodot: 0}
	for _, st := range steps {
		scores[st.engine] += st.points
	}
	diff := scores[EngineGodot] - scores[EngineHTML5]

	// Sort contributors by weight for the rationale.
	sort.Slice(steps, func(i, j int) bool { return steps[i].points > steps[j].points })
	var rationale []string
	for _, st := range steps {
		if len(rationale) >= 4 {
			break
		}
		rationale = append(rationale, fmt.Sprintf("[%s +%d] %s.", st.engine, st.points, st.why))
	}

	engine := EngineGodot
	switch {
	case diff <= -25:
		// HTML5 wins only on a decisive simple-game margin: many
		// simple signals stacked (simple archetype, few sprites, no
		// effects, low-end budget, short session). Anything less
		// one-sided defaults to Godot as the richer engine.
		engine = EngineHTML5
	case diff < 0:
		rationale = append(rationale,
			"HTML5 leads on simplicity, but not decisively; defaulting to Godot as the richer engine.")
	default:
		rationale = append(rationale,
			"Godot leads or scores are close; defaulting to Godot as the richer engine.")
	}
	if len(rationale) == 0 {
		rationale = []string{"No strong signals either way; defaulting to Godot."}
	}
	return Decision{Engine: engine, Rationale: rationale, Scores: scores}
}
