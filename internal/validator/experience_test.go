package validator

import (
	"strings"
	"testing"

	"doodlequest/internal/gameir"
)

const expDocJSON = `{
  "irVersion": "1.2",
  "capabilities": ["shell.v1", "render.sprite", "physics.bodies", "audio"],
  "game": {"id": "exp-game", "title": "Exp", "entryScene": "main", "orientation": "portrait",
           "sessionTargetSeconds": 30, "audience": "family"},
  "assets": [
    {"id": "hero", "kind": "image", "uri": "sprites/hero.png"},
    {"id": "heart", "kind": "image", "uri": "sprites/heart.png"}
  ],
  "scenes": [{
    "id": "main", "gravity": [0, 700], "worldSize": [390, 844], "pixelsPerMeter": 100,
    "entities": [
      {"id": "player", "kind": "player", "assetId": "hero", "position": [195, 700], "size": [60, 80],
       "components": ["movement"], "physics": {"body": "kinematic_position", "shape": "capsule"}},
      {"id": "heart", "kind": "pickup", "assetId": "heart", "position": [195, 240], "size": [30, 30],
       "components": ["collider", "collectible"], "physics": {"body": "dynamic", "shape": "ball", "initialVelocity": [0, 50]}},
      {"id": "heart2", "kind": "pickup", "assetId": "heart", "position": [90, 120], "size": [30, 30],
       "components": ["collider", "collectible"], "physics": {"body": "dynamic", "shape": "ball", "initialVelocity": [0, 50]}},
      {"id": "heart3", "kind": "pickup", "assetId": "heart", "position": [300, 60], "size": [30, 30],
       "components": ["collider", "collectible"], "physics": {"body": "dynamic", "shape": "ball", "initialVelocity": [0, 50]}}
    ]
  }],
  "inputBindings": [{"input": "drag", "action": "move"}],
  "rules": [
    {"id": "collect", "when": {"event": "collected", "target": "heart"},
     "actions": [{"type": "add_score", "value": 1}]},
    {"id": "win", "when": {"event": "score_reached", "value": 12},
     "actions": [{"type": "end_game", "value": true}]}
  ],
  "experience": {
    "archetype": "catcher",
    "controls": {"scheme": "drag_x", "hint": "Drag to move"},
    "objective": {"kind": "collect", "target": 12, "label": "Catch 12 hearts"},
    "hud": {"left": "objective_progress", "right": "none"},
    "start": {"mode": "tap_to_start"},
    "failure": {"mode": "gentle"},
    "pacing": {"targetSessionSec": 30, "firstRewardWithinSec": 10, "expectedCatchRate": 0.25,
               "ramp": [{"at": 0, "speedMul": 1}, {"at": 0.5, "speedMul": 1.2}, {"at": 0.85, "speedMul": 1.45}]},
    "milestones": [0.25, 0.5, 0.75, 1],
    "milestoneVisuals": {"0.25": "heart", "0.5": "heart", "0.75": "heart", "1": "heart"},
    "result": {"win": {"title": "Done!"}, "lose": {"title": "Keep going!"}},
    "feedback": {"haptics": "none", "juice": "standard"}
  }
}`

func decodeExp(t *testing.T, jsonDoc string) *gameir.Document {
	t.Helper()
	doc, err := gameir.DecodeAndValidate([]byte(jsonDoc))
	if err != nil {
		t.Fatalf("fixture must decode: %v", err)
	}
	return doc
}

func findCheck(checks []Check, id string) Check {
	for _, c := range checks {
		if c.CheckID == id {
			return c
		}
	}
	return Check{Status: "missing"}
}

func TestExperienceChecksHappyPath(t *testing.T) {
	checks := experienceChecks(decodeExp(t, expDocJSON), nil)
	for _, id := range []string{"experience.standardized", "experience.slots", "experience.balance",
		"experience.failure_model", "experience.ramp", "experience.milestones"} {
		c := findCheck(checks, id)
		if c.Status != StatusPass {
			t.Errorf("%s = %s (%s)", id, c.Status, c.Evidence)
		}
	}
}

func TestExperienceSupportsIndependentCollectProgressVariable(t *testing.T) {
	fixture := strings.Replace(expDocJSON,
		`"objective": {"kind": "collect", "target": 12, "label": "Catch 12 hearts"}`,
		`"objective": {"kind": "collect", "target": 12, "label": "Catch 12 hearts", "progressVariable": "hearts_collected"}`, 1)
	fixture = strings.Replace(fixture,
		`"actions": [{"type": "add_score", "value": 1}]`,
		`"actions": [{"type": "add_score", "value": 10}, {"type": "add_variable", "variable": "hearts_collected", "value": 1}]`, 1)
	doc := decodeExp(t, fixture)
	if doc.Experience.Objective.ProgressVariable != "hearts_collected" {
		t.Fatalf("progress variable = %q, want hearts_collected", doc.Experience.Objective.ProgressVariable)
	}
	if !strings.Contains(fixture, `"type": "add_variable"`) {
		t.Fatal("fixture should exercise the additive variable action")
	}
}

func TestExperienceBalanceUsesCustomObjectiveProgress(t *testing.T) {
	doc := &gameir.Document{
		Game: gameir.Game{ID: "counter-game", EntryScene: "main", SessionTargetSeconds: 20},
		Scenes: []gameir.Scene{{
			ID: "main", WorldSize: []float64{390, 844}, Gravity: []float64{0, 700},
			Entities: []gameir.Entity{
				{ID: "heart", Kind: "pickup", Position: []float64{50, 100}, Components: []string{"collider"}, Physics: &gameir.Physics{InitialVelocity: []float64{0, 50}}},
				{ID: "heart_blue", Kind: "pickup", Position: []float64{195, 100}, Components: []string{"collider"}, Physics: &gameir.Physics{InitialVelocity: []float64{0, 50}}},
				{ID: "heart_gold", Kind: "pickup", Position: []float64{340, 100}, Components: []string{"collider"}, Physics: &gameir.Physics{InitialVelocity: []float64{0, 50}}},
			},
		}},
		Experience: &gameir.Experience{
			Objective: gameir.ExpObjective{Kind: "collect", Target: 12, Label: "Catch 12 hearts", ProgressVariable: "hearts_collected"},
			Pacing:    gameir.ExpPacing{TargetSessionSec: 20, ExpectedCatchRate: 0.44},
		},
		Rules: []gameir.Rule{
			{ID: "pink", When: gameir.Condition{Event: "collected", Target: "heart"}, Actions: []gameir.Action{{Type: "add_score", Value: 10}, {Type: "add_variable", Variable: "hearts_collected", Value: 1}}},
			{ID: "blue", When: gameir.Condition{Event: "collected", Target: "heart_blue"}, Actions: []gameir.Action{{Type: "add_score", Value: 10}, {Type: "add_variable", Variable: "hearts_collected", Value: 1}}},
			{ID: "gold", When: gameir.Condition{Event: "collected", Target: "heart_gold"}, Actions: []gameir.Action{{Type: "add_score", Value: 50}}},
		},
	}
	check := expBalanceCheck(doc)
	if check.Status != StatusPass {
		t.Fatalf("custom objective balance = %s (%s), want pass", check.Status, check.Evidence)
	}
	if !strings.Contains(check.Evidence, "avg 0.67 objective units/catch") {
		t.Fatalf("balance evidence should use regular-heart progress rather than bonus score: %s", check.Evidence)
	}
}

func TestAddVariableRejectsMissingOrNonNumericArguments(t *testing.T) {
	fixture := strings.Replace(expDocJSON,
		`"objective": {"kind": "collect", "target": 12, "label": "Catch 12 hearts"}`,
		`"objective": {"kind": "collect", "target": 12, "label": "Catch 12 hearts", "progressVariable": "hearts_collected"}`, 1)
	fixture = strings.Replace(fixture,
		`"actions": [{"type": "add_score", "value": 1}]`,
		`"actions": [{"type": "add_variable", "variable": "hearts_collected", "value": 1}]`, 1)
	badValue := strings.Replace(fixture,
		`"variable": "hearts_collected", "value": 1`,
		`"variable": "hearts_collected", "value": "one"`, 1)
	if _, err := gameir.DecodeAndValidate([]byte(badValue)); err == nil {
		t.Fatal("add_variable should reject a non-numeric increment")
	}
	missingVariable := strings.Replace(fixture,
		`"variable": "hearts_collected", "value": 1`,
		`"value": 1`, 1)
	if _, err := gameir.DecodeAndValidate([]byte(missingVariable)); err == nil {
		t.Fatal("add_variable should reject a missing variable name")
	}
}

func TestExperienceChecksPreStandardGame(t *testing.T) {
	doc := decodeExp(t, strings.Replace(expDocJSON, `"irVersion": "1.2"`, `"irVersion": "1.1"`, 1))
	doc.Experience = nil
	checks := experienceChecks(doc, nil)
	if c := findCheck(checks, "experience.standardized"); c.Status != StatusFail {
		t.Fatalf("standardized = %s, want fail for a pre-1.2 game", c.Status)
	}
	if c := findCheck(checks, "experience.balance"); c.Status != StatusSkip {
		t.Fatalf("balance = %s, want skip", c.Status)
	}
}

func TestExperienceFailureContradictions(t *testing.T) {
	// Gentle declared, but the win rule is fine — add a LOSE end_game
	// rule and the declaration contradicts itself.
	bad := strings.Replace(expDocJSON, `{"id": "win"`, `{"id": "harsh", "when": {"event": "timer_elapsed", "value": 5},
     "actions": [{"type": "end_game", "value": false}]},
    {"id": "win"`, 1)
	checks := experienceChecks(decodeExp(t, bad), nil)
	if c := findCheck(checks, "experience.failure_model"); c.Status != StatusFail {
		t.Fatalf("gentle + end_game rule: failure_model = %s, want fail", c.Status)
	}

	// Sudden-death for a family audience.
	bad = strings.Replace(expDocJSON, `"failure": {"mode": "gentle"}`, `"failure": {"mode": "sudden-death"}`, 1)
	checks = experienceChecks(decodeExp(t, bad), nil)
	if c := findCheck(checks, "experience.failure_model"); c.Status != StatusFail {
		t.Fatalf("sudden-death + family: failure_model = %s, want fail", c.Status)
	}

	// Health mode without an invulnerability window.
	bad = strings.Replace(expDocJSON, `"failure": {"mode": "gentle"}`, `"failure": {"mode": "health", "health": 3}`, 1)
	checks = experienceChecks(decodeExp(t, bad), nil)
	if c := findCheck(checks, "experience.failure_model"); c.Status != StatusFail {
		t.Fatalf("health without invuln: failure_model = %s, want fail", c.Status)
	}
}

func TestExperienceBalanceOutOfBand(t *testing.T) {
	// Same economics, absurd session target: expected ~24s vs 300s.
	bad := strings.Replace(expDocJSON, `"targetSessionSec": 30`, `"targetSessionSec": 300`, 1)
	bad = strings.Replace(bad, `"sessionTargetSeconds": 30`, `"sessionTargetSeconds": 300`, 1)
	checks := experienceChecks(decodeExp(t, bad), nil)
	c := findCheck(checks, "experience.balance")
	if c.Status != StatusFail {
		t.Fatalf("balance = %s, want fail (evidence: %s)", c.Status, c.Evidence)
	}
	if !strings.Contains(c.Evidence, "recycle:") {
		t.Fatalf("balance evidence should show the recycle arithmetic: %s", c.Evidence)
	}
}

func TestExperienceBalanceSkipsStationaryPickups(t *testing.T) {
	static := strings.ReplaceAll(expDocJSON, `"gravity": [0, 700]`, `"gravity": [0, 0]`)
	static = strings.ReplaceAll(static, `, "physics": {"body": "dynamic", "shape": "ball", "initialVelocity": [0, 50]}`, "")
	c := findCheck(experienceChecks(decodeExp(t, static), nil), "experience.balance")
	if c.Status != StatusSkip || !strings.Contains(c.Evidence, "stationary pickups") {
		t.Fatalf("stationary pickup balance = %s (%s), want a game-aware probe skip", c.Status, c.Evidence)
	}
}
