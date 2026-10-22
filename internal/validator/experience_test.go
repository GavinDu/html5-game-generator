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
