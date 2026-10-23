package gameir

import (
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../examples/game-ir-v1/bobos-rainbow.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGoldenFixtureIsValid(t *testing.T) {
	if _, err := DecodeAndValidate(readFixture(t)); err != nil {
		t.Fatalf("golden fixture should validate: %v", err)
	}
}

func TestRejectsUnknownExecutableField(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"irVersion": "1.0"`, `"irVersion": "1.0", "executableScript": "print(1)"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unknown script field to be rejected")
	}
}

func TestRejectsPathTraversal(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"uri": "assets/field.png"`, `"uri": "assets/../secret.png"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}

func TestRejectsMissingAssetReference(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"assetId": "bobo"`, `"assetId": "missing"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected missing asset reference to be rejected")
	}
}

func TestRejectsUnsupportedAction(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"type": "add_score"`, `"type": "execute_script"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unsupported action to be rejected")
	}
}

func TestRejectsTrailingJSON(t *testing.T) {
	data := append(readFixture(t), []byte("{}")...)
	if _, err := DecodeAndValidate(data); err == nil {
		t.Fatal("expected trailing JSON value to be rejected")
	}
}

func TestRejectsUnsupportedVersion(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"irVersion": "1.0"`, `"irVersion": "2.0"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unsupported version to be rejected")
	}
}

const v11Doc = `{
  "irVersion": "1.1",
  "capabilities": ["physics.bodies", "physics.joints", "render.particles", "input.keyboard"],
  "game": {"id": "v11-game", "title": "V11", "entryScene": "main", "orientation": "portrait"},
  "assets": [
    {"id": "hero", "kind": "image", "uri": "sprites/hero.png"},
    {"id": "hero2", "kind": "image", "uri": "sprites/hero2.png"},
    {"id": "gem", "kind": "image", "uri": "sprites/gem.png"},
    {"id": "spark", "kind": "image", "uri": "sprites/spark.png"},
    {"id": "theme", "kind": "audio", "uri": "audio/theme.wav"}
  ],
  "scenes": [{
    "id": "main",
    "backgroundColor": "#10203a",
    "gravity": [0, 900],
    "pixelsPerMeter": 100,
    "layers": [{"id": "bg", "assetId": "hero", "parallax": [0.5, 0.25], "alpha": 0.8}],
    "emitters": [{"rate": 5, "tint": "#ffffff", "lifetime": [0.5, 1.5], "speed": [10, 40]}],
    "entities": [
      {"id": "player", "kind": "player", "assetId": "hero", "position": [195, 600], "size": [60, 80],
       "components": ["movement", "physics_body", "character_controller"],
       "physics": {"body": "kinematic_position", "shape": "capsule", "friction": 0.4,
                   "collisionGroups": {"memberships": [0], "filter": [1, 2]}},
       "character": {"offset": 4, "maxSlopeClimbDegrees": 45, "snapToGroundDistance": 12},
       "render": {"tint": "#ffeecc", "animation": {"frames": ["hero", "hero2"], "fps": 8},
                  "particles": {"rate": 12, "tint": "#ffd94d"}},
       "tweens": [{"property": "scale", "from": 1, "to": 1.08, "durationMs": 800, "repeat": -1, "yoyo": true, "easing": "sine"}]},
      {"id": "anchor", "kind": "obstacle", "position": [195, 120], "size": [20, 20],
       "components": ["collider"], "physics": {"body": "fixed", "shape": "cuboid"}},
      {"id": "gem", "kind": "pickup", "assetId": "gem", "position": [195, 300], "size": [30, 30],
       "components": ["collider", "collectible"],
       "physics": {"body": "dynamic", "shape": "ball", "density": 0.8, "restitution": 0.5,
                   "frictionCombineRule": "max", "ccd": true, "initialVelocity": [0, 30]},
       "render": {"filters": [{"type": "color_matrix", "operation": "saturate", "value": 1.4}]}},
      {"id": "rock", "kind": "obstacle", "position": [80, 300], "size": [36, 36],
       "components": ["collider"],
       "physics": {"body": "dynamic", "shape": "convex", "points": [[-18, -12], [18, -12], [0, 18]]}},
      {"id": "plate", "kind": "decoration", "position": [195, 800], "size": [300, 24],
       "components": [], "render": {"shape": {"type": "rounded_rect", "fill": "#3355aa", "cornerRadius": 10}}},
      {"id": "label", "kind": "text", "position": [195, 60],
       "components": ["text_label"], "render": {"text": {"content": "Go!", "fontSize": 28, "fill": "#ffffff"}}}
    ],
    "templates": [
      {"id": "spark", "kind": "decoration", "assetId": "spark", "position": [0, 0], "size": [24, 24], "components": []}
    ],
    "joints": [
      {"id": "gem-spring", "type": "spring", "bodyA": "anchor", "bodyB": "gem",
       "anchorA": [0, 10], "anchorB": [0, -10], "restLength": 60, "stiffness": 30, "damping": 3}
    ]
  }],
  "inputBindings": [
    {"action": "drag_player", "input": "drag"},
    {"action": "move_left", "input": "keyboard", "key": "ArrowLeft"}
  ],
  "rules": [
    {"id": "gem-hit", "when": {"event": "collected", "source": "player", "target": "gem"},
     "actions": [
       {"type": "apply_impulse", "target": "player", "value": [0, -300]},
       {"type": "emit_particles", "target": "gem", "value": 12},
       {"type": "set_asset", "target": "gem", "assetId": "spark"}
     ]},
    {"id": "start", "when": {"event": "game_start"},
     "actions": [
       {"type": "spawn_entity", "target": "spark"},
       {"type": "set_velocity", "target": "gem", "value": [10, 20]},
       {"type": "camera_shake", "value": 5},
       {"type": "change_health", "value": -1}
     ]}
  ]
}`

func TestV11ExpandedDocumentIsValid(t *testing.T) {
	if _, err := DecodeAndValidate([]byte(v11Doc)); err != nil {
		t.Fatalf("v1.1 expanded document should validate: %v", err)
	}
}

func TestV11RejectsUnknownCapability(t *testing.T) {
	data := strings.Replace(v11Doc, `"physics.bodies"`, `"physics.magic"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unknown capability to be rejected")
	}
}

func TestV11RejectsJointWithMissingBody(t *testing.T) {
	data := strings.Replace(v11Doc, `"bodyB": "gem"`, `"bodyB": "ghost"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected joint with missing body to be rejected")
	}
}

func TestV11RejectsSpawnOfNonTemplate(t *testing.T) {
	data := strings.Replace(v11Doc, `{"type": "spawn_entity", "target": "spark"}`, `{"type": "spawn_entity", "target": "gem"}`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected spawn of a non-template to be rejected")
	}
}

func TestV11RejectsBadPhysicsShape(t *testing.T) {
	data := strings.Replace(v11Doc, `"shape": "ball"`, `"shape": "torus"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected unsupported physics shape to be rejected")
	}
}

func TestV11RejectsScalarImpulse(t *testing.T) {
	data := strings.Replace(v11Doc, `"value": [0, -300]`, `"value": 5`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected scalar impulse value to be rejected")
	}
}

func TestRejectsDuplicateEntityIDs(t *testing.T) {
	data := strings.Replace(string(readFixture(t)), `"id": "kitten",
          "kind": "decoration",
          "assetId": "kitten"`, `"id": "player",
          "kind": "decoration",
          "assetId": "kitten"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("expected duplicate entity ID to be rejected")
	}
}

const expBlock = `,
  "experience": {
    "archetype": "catcher",
    "controls": {"scheme": "drag_x", "hint": "Drag to move"},
    "objective": {"kind": "collect", "target": 10, "label": "Catch 10 gems"},
    "hud": {"left": "objective_progress", "right": "health"},
    "start": {"mode": "tap_to_start"},
    "failure": {"mode": "health", "health": 3, "invulnerableMs": 1000},
    "pacing": {"targetSessionSec": 60, "firstRewardWithinSec": 10, "expectedCatchRate": 0.6,
               "ramp": [{"at": 0, "spawnIntervalMs": 900, "speedMul": 1},
                        {"at": 0.5, "spawnIntervalMs": 700, "speedMul": 1.2},
                        {"at": 0.85, "spawnIntervalMs": 550, "speedMul": 1.45}]},
    "milestones": [0.25, 0.5, 0.75, 1],
    "result": {"win": {"title": "Done!", "stars": {"by": "time", "thresholdsSec": [60, 40, 25]}},
               "lose": {"title": "So close!"}},
    "leaderboard": {"metric": "time", "order": "asc"},
    "feedback": {"haptics": "none", "juice": "standard"}
  }`

func v12Doc(t *testing.T) string {
	t.Helper()
	data := strings.Replace(v11Doc, `"irVersion": "1.1"`, `"irVersion": "1.2"`, 1)
	data = strings.Replace(data, `"input.keyboard"]`, `"input.keyboard", "shell.v1"]`, 1)
	// Inject the experience block just before the closing brace of the
	// document: the v11Doc ends with the scenes array then }.
	idx := strings.LastIndex(data, "}")
	return data[:idx] + expBlock + data[idx:]
}

func TestV12ExperienceBlockValid(t *testing.T) {
	doc, err := DecodeAndValidate([]byte(v12Doc(t)))
	if err != nil {
		t.Fatalf("valid 1.2 doc rejected: %v", err)
	}
	if doc.Experience == nil || doc.Experience.Archetype != "catcher" {
		t.Fatal("experience block not decoded")
	}
}

func TestV12RequiresExperienceBlock(t *testing.T) {
	data := strings.Replace(v11Doc, `"irVersion": "1.1"`, `"irVersion": "1.2"`, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("1.2 without an experience block must be rejected")
	}
}

func TestExperienceRequiresShellCapability(t *testing.T) {
	data := v12Doc(t)
	data = strings.Replace(data, `, "shell.v1"`, ``, 1)
	if _, err := DecodeAndValidate([]byte(data)); err == nil {
		t.Fatal("experience block without shell.v1 must be rejected")
	}
}

func TestExperienceRejectsBadSchemeAndRamp(t *testing.T) {
	bad := strings.Replace(v12Doc(t), `"scheme": "drag_x"`, `"scheme": "teleport"`, 1)
	if _, err := DecodeAndValidate([]byte(bad)); err == nil {
		t.Fatal("unknown control scheme must be rejected")
	}
	bad = strings.Replace(v12Doc(t), `"at": 0.85`, `"at": 0.4`, 1)
	if _, err := DecodeAndValidate([]byte(bad)); err == nil {
		t.Fatal("non-monotonic ramp must be rejected")
	}
	bad = strings.Replace(v12Doc(t), `"thresholdsSec": [60, 40, 25]`, `"thresholdsSec": [25, 40, 60]`, 1)
	if _, err := DecodeAndValidate([]byte(bad)); err == nil {
		t.Fatal("ascending star thresholds must be rejected")
	}
}

// The vocabulary is what the runtime implements (2026-10-23, Game IR
// commitment): kinds that were declarable but never executed — video
// assets, swipe/tilt/joystick inputs — must now fail validation.
func TestRejectsUnimplementedKinds(t *testing.T) {
	base := string(readFixture(t))
	cases := map[string]string{
		"video asset": strings.Replace(base, `"kind": "image"`, `"kind": "video"`, 1),
		"tilt input":  strings.Replace(base, `"input": "drag"`, `"input": "tilt"`, 1),
	}
	for name, data := range cases {
		if _, err := DecodeAndValidate([]byte(data)); err == nil {
			t.Errorf("%s: expected rejection, got valid", name)
		}
	}
}
