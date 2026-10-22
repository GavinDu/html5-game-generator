package validator

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"doodlequest/internal/gameir"
)

// Experience-standard checks (IR 1.2). They read the game's own
// game-ir.json from the game directory: a directory without one gets
// honest skips (the design-input pipeline validates games before an IR
// exists). Everything here is computed from the document — the
// balance check in particular shows its arithmetic in the evidence,
// because a quality claim nobody can recompute is just a vibe.

func readGameIR(dir string) (*gameir.Document, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "game-ir.json"))
	if err != nil {
		return nil, nil // absent is not an error: checks skip
	}
	return gameir.DecodeAndValidate(raw)
}

func experienceChecks(doc *gameir.Document, irErr error) []Check {
	skipAll := func(reason string) []Check {
		ids := []string{"experience.standardized", "experience.slots", "experience.balance",
			"experience.failure_model", "experience.ramp", "experience.milestones"}
		out := []Check{}
		for _, id := range ids {
			out = append(out, mk(id, StatusSkip, "experience block evaluated", "not evaluated", reason, "game-ir.json", ""))
		}
		return out
	}
	if irErr != nil {
		return skipAll(fmt.Sprintf("game-ir.json failed structural validation: %v", irErr))
	}
	if doc == nil {
		return skipAll("no game-ir.json in the game directory")
	}
	exp := doc.Experience
	if exp == nil {
		return []Check{
			mk("experience.standardized", StatusFail, "experience block present",
				"absent (irVersion "+doc.IRVersion+")",
				"the game predates the IR 1.2 experience standard: organization and control are emergent, not declared",
				"game-ir.json", "add an experience block: archetype, controls.scheme, objective, failure mode, pacing"),
			mk("experience.slots", StatusSkip, "", "", "no experience block", "game-ir.json", ""),
			mk("experience.balance", StatusSkip, "", "", "no experience block", "game-ir.json", ""),
			mk("experience.failure_model", StatusSkip, "", "", "no experience block", "game-ir.json", ""),
			mk("experience.ramp", StatusSkip, "", "", "no experience block", "game-ir.json", ""),
			mk("experience.milestones", StatusSkip, "", "", "no experience block", "game-ir.json", ""),
		}
	}
	return []Check{
		mk("experience.standardized", StatusPass, "experience block present", "present",
			fmt.Sprintf("archetype %s, scheme %s, objective %s", exp.Archetype, exp.Controls.Scheme, exp.Objective.Kind),
			"game-ir.json", ""),
		expSlotsCheck(doc),
		expBalanceCheck(doc),
		expFailureCheck(doc),
		expRampCheck(doc),
		expMilestonesCheck(doc),
	}
}

func entryScene(doc *gameir.Document) *gameir.Scene {
	for i := range doc.Scenes {
		if doc.Scenes[i].ID == doc.Game.EntryScene {
			return &doc.Scenes[i]
		}
	}
	if len(doc.Scenes) > 0 {
		return &doc.Scenes[0]
	}
	return nil
}

func expSlotsCheck(doc *gameir.Document) Check {
	id := "experience.slots"
	sc := entryScene(doc)
	if sc == nil {
		return mk(id, StatusSkip, "", "", "no scenes", "game-ir.json", "")
	}
	kinds := map[string]int{}
	hazardKinds := map[string]bool{}
	pickups := 0
	player := false
	for _, e := range append(append([]gameir.Entity{}, sc.Entities...), sc.Templates...) {
		kinds[e.Kind]++
		switch e.Kind {
		case "player":
			player = true
		case "pickup":
			pickups++
		case "obstacle", "enemy":
			hazardKinds[e.ID] = true
		}
		for _, c := range e.Components {
			if c == "collectible" {
				pickups++
			}
		}
	}
	exp := doc.Experience
	switch exp.Archetype {
	case "catcher":
		if !player {
			return mk(id, StatusFail, "player + >=1 collectible kind", "no player entity", "catcher slot spec: the player sprite slot is empty", "game-ir.json", "add a player entity")
		}
		if pickups < 1 {
			return mk(id, StatusFail, ">=1 collectible kind", "0 collectibles", "catcher slot spec: nothing to catch", "game-ir.json", "add a pickup entity or template")
		}
		if len(hazardKinds) > 2 {
			return mk(id, StatusFail, "<=2 hazard kinds", fmt.Sprintf("%d hazard kinds", len(hazardKinds)), "catcher slot spec allows 0-2 hazard kinds", "game-ir.json", "merge hazard kinds")
		}
		return mk(id, StatusPass, "catcher slots filled", fmt.Sprintf("player + %d collectibles + %d hazard kinds", pickups, len(hazardKinds)), "slot spec satisfied", "game-ir.json", "")
	case "dodger":
		if !player || len(hazardKinds) < 1 {
			return mk(id, StatusFail, "player + >=1 hazard kind", fmt.Sprintf("player=%v hazards=%d", player, len(hazardKinds)), "dodger slot spec: a player and something to dodge", "game-ir.json", "add a player / hazard entity")
		}
		return mk(id, StatusPass, "dodger slots filled", fmt.Sprintf("player + %d hazard kinds + %d collectibles", len(hazardKinds), pickups), "slot spec satisfied", "game-ir.json", "")
	default:
		if !player {
			return mk(id, StatusFail, "player entity", "none", exp.Archetype+" slot spec: the player slot is empty", "game-ir.json", "add a player entity")
		}
		return mk(id, StatusPass, "generic slots filled", "player present", "archetype "+exp.Archetype+" uses the generic slot spec in v1", "game-ir.json", "")
	}
}

// expBalanceCheck computes the expected win time from the game's own
// economics. Two production models exist on this platform:
//   - stream-spawned games (rules use spawn_entity): spawns arrive at
//     the ramp's mean interval; a declared fraction are caught.
//   - recycle games (the shipped family): N pickups fall on a cycle of
//     length T = fall time through the world under gravity; the max
//     collect rate is N/T and the declared catch rate discounts it.
func expBalanceCheck(doc *gameir.Document) Check {
	id := "experience.balance"
	exp := doc.Experience
	if exp.Objective.Kind != "collect" || exp.Objective.Target < 1 {
		return mk(id, StatusSkip, "", "", "balance model covers collect objectives", "game-ir.json", "")
	}
	target := exp.Objective.Target
	session := exp.Pacing.TargetSessionSec
	if session == 0 {
		session = doc.Game.SessionTargetSeconds
	}
	if session == 0 {
		return mk(id, StatusSkip, "", "", "no session target declared (pacing.targetSessionSec or game.sessionTargetSeconds)", "game-ir.json", "")
	}
	catchRate := exp.Pacing.ExpectedCatchRate
	rateNote := "declared"
	if catchRate == 0 {
		catchRate = 0.4
		rateNote = "default 0.4 (no expectedCatchRate declared)"
	}
	stream := false
	for _, r := range doc.Rules {
		for _, a := range r.Actions {
			if a.Type == "spawn_entity" {
				stream = true
			}
		}
	}
	var expected float64
	var model string
	if stream {
		sum, n := 0, 0
		for _, s := range exp.Pacing.Ramp {
			if s.SpawnIntervalMs > 0 {
				sum += s.SpawnIntervalMs
				n++
			}
		}
		if n == 0 {
			return mk(id, StatusSkip, "", "", "stream-spawned game without ramp spawn intervals — economics unknown", "game-ir.json", "")
		}
		avg := float64(sum) / float64(n)
		expected = float64(target) / (catchRate * 1000.0 / avg)
		model = fmt.Sprintf("stream: mean spawn interval %.0fms, catch rate %.2f (%s)", avg, catchRate, rateNote)
	} else {
		sc := entryScene(doc)
		if sc == nil {
			return mk(id, StatusSkip, "", "", "no scenes", "game-ir.json", "")
		}
		var pickups []gameir.Entity
		for _, e := range sc.Entities {
			if e.Kind == "pickup" {
				pickups = append(pickups, e)
			}
		}
		if len(pickups) == 0 {
			return mk(id, StatusSkip, "", "", "no concurrent pickups in the entry scene — recycle economics unknown", "game-ir.json", "")
		}
		h := 844.0
		if len(sc.WorldSize) == 2 && sc.WorldSize[1] > 0 {
			h = sc.WorldSize[1]
		}
		vSum := 0.0
		for _, p := range pickups {
			v := 50.0
			if p.Physics != nil && len(p.Physics.InitialVelocity) == 2 {
				v = math.Abs(p.Physics.InitialVelocity[1])
			}
			vSum += v
		}
		v := vSum / float64(len(pickups))
		g := 0.0
		if len(sc.Gravity) == 2 {
			g = math.Abs(sc.Gravity[1])
		}
		var fall float64
		if g > 0 {
			fall = (-v + math.Sqrt(v*v+2*g*h)) / g
		} else if v > 0 {
			fall = h / v
		} else {
			return mk(id, StatusSkip, "", "", "pickups neither fall nor are gravity-driven — cycle time unknown", "game-ir.json", "")
		}
		// Weighted scoring: collect rules award different points per
		// pickup kind (a gold heart is worth 3). The economics run on
		// points per second, so average the award over the concurrent
		// pickups, read from the game's own collected rules.
		pointsFor := func(entID string) float64 {
			for _, r := range doc.Rules {
				if r.When.Event != "collected" || (r.When.Target != entID && r.When.Target != "") {
					continue
				}
				for _, a := range r.Actions {
					if a.Type == "add_score" {
						if fv, ok := a.Value.(float64); ok && fv > 0 {
							return fv
						}
						return 1
					}
				}
			}
			return 1
		}
		ptsSum := 0.0
		for _, p := range pickups {
			ptsSum += pointsFor(p.ID)
		}
		avgPts := ptsSum / float64(len(pickups))
		maxRate := float64(len(pickups)) / fall
		expected = float64(target) / (catchRate * maxRate * avgPts)
		model = fmt.Sprintf("recycle: %d pickups, fall cycle %.2fs (v=%.0fpx/s, g=%.0f, h=%.0f), max rate %.2f catches/s, avg %.1f pts/catch, catch rate %.2f (%s)",
			len(pickups), fall, v, g, h, maxRate, avgPts, catchRate, rateNote)
	}
	ratio := expected / float64(session)
	ev := fmt.Sprintf("%s -> expected win %.1fs vs session target %ds (ratio %.2f, band 0.70-1.30)", model, expected, session, ratio)
	if ratio >= 0.7 && ratio <= 1.3 {
		return mk(id, StatusPass, "expected win within 0.7-1.3x of session target", fmt.Sprintf("ratio %.2f", ratio), ev, "game-ir.json", "")
	}
	return mk(id, StatusFail, "expected win within 0.7-1.3x of session target", fmt.Sprintf("ratio %.2f", ratio), ev, "game-ir.json",
		"tune the objective target, the fall speeds / spawn intervals, or declare the session target the economics actually produce")
}

func expFailureCheck(doc *gameir.Document) Check {
	id := "experience.failure_model"
	exp := doc.Experience
	mode := exp.Failure.Mode
	if mode == "" {
		mode = "health"
	}
	if mode == "sudden-death" && (doc.Game.Audience == "family" || doc.Game.Audience == "kids") {
		return mk(id, StatusFail, "no sudden-death for family/kids audiences", "sudden-death + audience "+doc.Game.Audience,
			"one mistake ends the run in a game aimed at families — the standard reserves sudden-death for declared challenge games",
			"game-ir.json", "use the health model (3 hearts + invulnerability window) or gentle mode")
	}
	if mode == "gentle" || mode == "none" {
		for _, r := range doc.Rules {
			for _, a := range r.Actions {
				// A gentle game may still end in a WIN (end_game true);
				// what contradicts gentle is a rule that ends it in a
				// loss (end_game false).
				if a.Type == "end_game" {
					if lost, ok := a.Value.(bool); ok && !lost {
						return mk(id, StatusFail, "gentle games never end the run in a loss", "rule "+r.ID+" ends the game in a loss",
							"failure mode declares gentle but a rule ends the game in a loss — the declaration and the rules contradict",
							"game-ir.json", "remove the losing end_game rule or declare the real failure model")
					}
				}
			}
		}
		return mk(id, StatusPass, "gentle failure model, consistently rule-free", "declared "+mode, "no end_game rules — a no-fail game by declaration, not by accident", "game-ir.json", "")
	}
	if mode == "health" && exp.Failure.InvulnerableMs < 250 {
		return mk(id, StatusFail, "health mode declares an invulnerability window >= 250ms", fmt.Sprintf("invulnerableMs=%d", exp.Failure.InvulnerableMs),
			"without an invulnerability window, chained contact damage drains all health in consecutive frames — the unfair-damage failure the standard exists to prevent",
			"game-ir.json", "declare failure.invulnerableMs (1000 is the platform default)")
	}
	return mk(id, StatusPass, "failure model declared and consistent", "mode "+mode,
		fmt.Sprintf("mode %s, health %d, invulnerableMs %d", mode, exp.Failure.Health, exp.Failure.InvulnerableMs), "game-ir.json", "")
}

func expRampCheck(doc *gameir.Document) Check {
	id := "experience.ramp"
	exp := doc.Experience
	if len(exp.Pacing.Ramp) == 0 {
		return mk(id, StatusFail, "pacing ramp declared", "absent",
			"no ramp: difficulty is flat across the session (or emergent from physics nobody declared)",
			"game-ir.json", "declare pacing.ramp steps with speedMul rising over normalized session time")
	}
	last := exp.Pacing.Ramp[len(exp.Pacing.Ramp)-1]
	first := exp.Pacing.Ramp[0]
	mul := func(s gameir.ExpRampStep) float64 {
		if s.SpeedMul == 0 {
			return 1
		}
		return s.SpeedMul
	}
	if mul(last) <= mul(first) {
		return mk(id, StatusFail, "ramp ends faster than it starts", fmt.Sprintf("speedMul %.2f -> %.2f", mul(first), mul(last)),
			"a ramp whose speed multiplier never rises is decoration, not pacing", "game-ir.json", "raise the final step's speedMul")
	}
	return mk(id, StatusPass, "pacing ramp declared and rising", fmt.Sprintf("%d steps, speedMul %.2f -> %.2f", len(exp.Pacing.Ramp), mul(first), mul(last)),
		"monotonicity of at is enforced structurally by gameir", "game-ir.json", "")
}

func expMilestonesCheck(doc *gameir.Document) Check {
	id := "experience.milestones"
	exp := doc.Experience
	if len(exp.Milestones) == 0 {
		return mk(id, StatusFail, "milestones declared", "absent",
			"no declared progress milestones — whether the player can see progress depends on hand-written rules per game",
			"game-ir.json", "declare milestones as fractions of the objective (e.g. [0.25, 0.5, 0.75, 1]) with milestoneVisuals")
	}
	if len(exp.MilestoneVisuals) == 0 {
		return mk(id, StatusFail, "milestones have declared visuals", "milestoneVisuals absent",
			"milestones without a visual mapping are numbers nobody sees", "game-ir.json", "map each milestone fraction to an asset or entity id")
	}
	known := map[string]bool{}
	for _, a := range doc.Assets {
		known[a.ID] = true
	}
	for _, sc := range doc.Scenes {
		for _, e := range append(append([]gameir.Entity{}, sc.Entities...), sc.Templates...) {
			known[e.ID] = true
		}
	}
	for frac, ref := range exp.MilestoneVisuals {
		if !known[ref] {
			return mk(id, StatusFail, "milestone visuals resolve", "milestone "+frac+" -> unknown "+ref,
				"milestoneVisuals must reference a declared asset or entity", "game-ir.json", "fix the milestoneVisuals mapping")
		}
	}
	return mk(id, StatusPass, "milestones declared with visuals", fmt.Sprintf("%d milestones, %d visuals", len(exp.Milestones), len(exp.MilestoneVisuals)),
		"progress is visible by construction", "game-ir.json", "")
}
