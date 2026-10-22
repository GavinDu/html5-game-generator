// Package gameir defines and validates the engine-independent, declarative Game IR.
// It deliberately excludes executable scripts: game behavior is expressed using
// allowlisted events and actions interpreted by a preinstalled runtime.
package gameir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

const Version = "1.0"

// Version11 is the additive expansion interpreted by the PixiJS +
// Rapier runtime: rendering vocabulary (tint/alpha/blend/animation/
// shape/text/filters/particles), first-class physics (bodies,
// colliders, joints, character controller), tweens, and a declared
// capability manifest. Version 1.0 documents remain valid.
const Version11 = "1.1"

// Version12 adds the experience block: the platform shell contract
// (archetype, control scheme, objective, failure model, pacing ramp,
// result/leaderboard presentation). A 1.2 document MUST carry the
// block and declare the "shell.v1" capability; earlier versions MAY
// carry the block (additive), and the runtime shell engages whenever
// the block is present.
const Version12 = "1.2"

var supportedVersions = enum(Version, Version11, Version12)

const (
	MaxAssets        = 256
	MaxScenes        = 64
	MaxEntities      = 500
	MaxTotalEntities = 2000
	MaxRules         = 256
	MaxInputBindings = 32
	MaxJoints        = 128
	MaxCapabilities  = 32
)

type Document struct {
	IRVersion     string         `json:"irVersion"`
	Capabilities  []string       `json:"capabilities,omitempty"`
	Game          Game           `json:"game"`
	Assets        []Asset        `json:"assets"`
	Scenes        []Scene        `json:"scenes"`
	InputBindings []InputBinding `json:"inputBindings"`
	Rules         []Rule         `json:"rules"`
	Experience    *Experience    `json:"experience,omitempty"`
	Metadata      Metadata       `json:"metadata,omitempty"`
}

type Game struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	EntryScene           string `json:"entryScene"`
	Orientation          string `json:"orientation"`
	SessionTargetSeconds int    `json:"sessionTargetSeconds,omitempty"`
	Audience             string `json:"audience,omitempty"`
}

type Asset struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Source   string `json:"source,omitempty"`
	AltText  string `json:"altText,omitempty"`
}

type Scene struct {
	ID                string            `json:"id"`
	BackgroundAssetID string            `json:"backgroundAssetId,omitempty"`
	BackgroundColor   string            `json:"backgroundColor,omitempty"`
	Gravity           []float64         `json:"gravity,omitempty"`
	WorldSize         []float64         `json:"worldSize,omitempty"`
	PixelsPerMeter    *float64          `json:"pixelsPerMeter,omitempty"`
	Layers            []Layer           `json:"layers,omitempty"`
	Entities          []Entity          `json:"entities"`
	Templates         []Entity          `json:"templates,omitempty"`
	Joints            []Joint           `json:"joints,omitempty"`
	Emitters          []ParticleEmitter `json:"emitters,omitempty"`
}

// Layer is a parallax plane rendered behind (zIndex < 0) or above the
// entity plane. Parallax is the fraction of camera motion the layer
// follows (1 = locked to the world, 0 = locked to the screen).
type Layer struct {
	ID       string    `json:"id"`
	AssetID  string    `json:"assetId,omitempty"`
	Parallax []float64 `json:"parallax,omitempty"`
	Tint     string    `json:"tint,omitempty"`
	Alpha    *float64  `json:"alpha,omitempty"`
	ZIndex   int       `json:"zIndex,omitempty"`
}

// ParticleEmitter describes a runtime-owned particle system (Pixi core
// ships the container, not emitter behavior). Angles are degrees in
// screen space: 0 = right, 90 = down. Ranges are [min, max].
type ParticleEmitter struct {
	TextureAssetID string    `json:"textureAssetId,omitempty"`
	Rate           *float64  `json:"rate,omitempty"`
	Burst          *float64  `json:"burst,omitempty"`
	Lifetime       []float64 `json:"lifetime,omitempty"`
	Speed          []float64 `json:"speed,omitempty"`
	Direction      *float64  `json:"direction,omitempty"`
	Spread         *float64  `json:"spread,omitempty"`
	Gravity        *float64  `json:"gravity,omitempty"`
	StartScale     *float64  `json:"startScale,omitempty"`
	EndScale       *float64  `json:"endScale,omitempty"`
	StartAlpha     *float64  `json:"startAlpha,omitempty"`
	EndAlpha       *float64  `json:"endAlpha,omitempty"`
	Tint           string    `json:"tint,omitempty"`
	BlendMode      string    `json:"blendMode,omitempty"`
	MaxParticles   int       `json:"maxParticles,omitempty"`
}

// Joint connects two entity bodies in the same scene. Anchors are
// local logical-px offsets from each body's center. Limits are radians
// for revolute joints and logical px for prismatic joints.
type Joint struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	BodyA      string      `json:"bodyA"`
	BodyB      string      `json:"bodyB"`
	AnchorA    []float64   `json:"anchorA,omitempty"`
	AnchorB    []float64   `json:"anchorB,omitempty"`
	Axis       []float64   `json:"axis,omitempty"`
	Limits     []float64   `json:"limits,omitempty"`
	Motor      *JointMotor `json:"motor,omitempty"`
	Length     *float64    `json:"length,omitempty"`
	RestLength *float64    `json:"restLength,omitempty"`
	Stiffness  *float64    `json:"stiffness,omitempty"`
	Damping    *float64    `json:"damping,omitempty"`
}

type JointMotor struct {
	Mode      string   `json:"mode"`
	Target    float64  `json:"target"`
	MaxForce  *float64 `json:"maxForce,omitempty"`
	Stiffness *float64 `json:"stiffness,omitempty"`
	Damping   *float64 `json:"damping,omitempty"`
}

type Entity struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	AssetID    string         `json:"assetId,omitempty"`
	Position   []float64      `json:"position"`
	Size       []float64      `json:"size,omitempty"`
	Rotation   float64        `json:"rotation,omitempty"`
	ZIndex     int            `json:"zIndex,omitempty"`
	Components []string       `json:"components"`
	Properties map[string]any `json:"properties,omitempty"`
	Behavior   *Behavior      `json:"behavior,omitempty"`
	Render     *Render        `json:"render,omitempty"`
	Physics    *Physics       `json:"physics,omitempty"`
	Character  *Character     `json:"character,omitempty"`
	Tweens     []Tween        `json:"tweens,omitempty"`
}

// Render is the PixiJS display vocabulary: transform styling, sprite
// animation, vector shapes, styled text, core filters, and an attached
// particle emitter. Colors are CSS hex strings ("#rrggbb").
type Render struct {
	Tint      string           `json:"tint,omitempty"`
	Alpha     *float64         `json:"alpha,omitempty"`
	BlendMode string           `json:"blendMode,omitempty"`
	Anchor    []float64        `json:"anchor,omitempty"`
	Scale     []float64        `json:"scale,omitempty"`
	FlipX     bool             `json:"flipX,omitempty"`
	FlipY     bool             `json:"flipY,omitempty"`
	Visible   *bool            `json:"visible,omitempty"`
	Animation *Animation       `json:"animation,omitempty"`
	Shape     *Shape           `json:"shape,omitempty"`
	Text      *TextSpec        `json:"text,omitempty"`
	Filters   []Filter         `json:"filters,omitempty"`
	Particles *ParticleEmitter `json:"particles,omitempty"`
}

type Animation struct {
	Frames   []string `json:"frames"`
	FPS      *float64 `json:"fps,omitempty"`
	Loop     *bool    `json:"loop,omitempty"`
	Autoplay *bool    `json:"autoplay,omitempty"`
	PingPong bool     `json:"pingPong,omitempty"`
}

type Shape struct {
	Type         string      `json:"type"`
	Fill         string      `json:"fill,omitempty"`
	Stroke       string      `json:"stroke,omitempty"`
	StrokeWidth  *float64    `json:"strokeWidth,omitempty"`
	CornerRadius *float64    `json:"cornerRadius,omitempty"`
	Points       [][]float64 `json:"points,omitempty"`
}

type TextSpec struct {
	Content       string   `json:"content"`
	FontFamily    string   `json:"fontFamily,omitempty"`
	FontSize      *float64 `json:"fontSize,omitempty"`
	Fill          string   `json:"fill,omitempty"`
	FontWeight    string   `json:"fontWeight,omitempty"`
	Align         string   `json:"align,omitempty"`
	Stroke        string   `json:"stroke,omitempty"`
	StrokeWidth   *float64 `json:"strokeWidth,omitempty"`
	WordWrapWidth *float64 `json:"wordWrapWidth,omitempty"`
}

// Filter is one of the five PixiJS core filters. For color_matrix,
// Operation names the ColorMatrixFilter method and Value its amount.
type Filter struct {
	Type      string   `json:"type"`
	Value     *float64 `json:"value,omitempty"`
	Operation string   `json:"operation,omitempty"`
	Scale     *float64 `json:"scale,omitempty"`
	AssetID   string   `json:"assetId,omitempty"`
}

// Physics is the Rapier vocabulary for one entity body + collider.
// Sizes come from the entity size unless Points define the shape.
// Velocities are logical px/s; angles are radians unless suffixed.
type Physics struct {
	Body                   string           `json:"body"`
	Shape                  string           `json:"shape,omitempty"`
	Points                 [][]float64      `json:"points,omitempty"`
	Density                *float64         `json:"density,omitempty"`
	Friction               *float64         `json:"friction,omitempty"`
	Restitution            *float64         `json:"restitution,omitempty"`
	FrictionCombineRule    string           `json:"frictionCombineRule,omitempty"`
	RestitutionCombineRule string           `json:"restitutionCombineRule,omitempty"`
	Sensor                 bool             `json:"sensor,omitempty"`
	GravityScale           *float64         `json:"gravityScale,omitempty"`
	LinearDamping          *float64         `json:"linearDamping,omitempty"`
	AngularDamping         *float64         `json:"angularDamping,omitempty"`
	CCD                    bool             `json:"ccd,omitempty"`
	LockRotation           bool             `json:"lockRotation,omitempty"`
	LockTranslation        []bool           `json:"lockTranslation,omitempty"`
	InitialVelocity        []float64        `json:"initialVelocity,omitempty"`
	AngularVelocity        *float64         `json:"angularVelocity,omitempty"`
	CollisionGroups        *CollisionGroups `json:"collisionGroups,omitempty"`
	ActiveEvents           *bool            `json:"activeEvents,omitempty"`
	CanSleep               *bool            `json:"canSleep,omitempty"`
	Dominance              *int             `json:"dominance,omitempty"`
}

// CollisionGroups packs Rapier's 16 membership bits + 16 filter bits.
// Group numbers are 0..15; an interaction happens only when each
// body's membership is in the other body's filter.
type CollisionGroups struct {
	Memberships []int `json:"memberships"`
	Filter      []int `json:"filter"`
}

// Character configures Rapier's KinematicCharacterController for a
// kinematic player body: slope handling, autostep, and ground snap.
// Distances are logical px, angles are degrees.
type Character struct {
	Offset                       *float64 `json:"offset,omitempty"`
	MaxSlopeClimbDegrees         *float64 `json:"maxSlopeClimbDegrees,omitempty"`
	MinSlopeSlideDegrees         *float64 `json:"minSlopeSlideDegrees,omitempty"`
	AutostepMaxHeight            *float64 `json:"autostepMaxHeight,omitempty"`
	AutostepMinWidth             *float64 `json:"autostepMinWidth,omitempty"`
	SnapToGroundDistance         *float64 `json:"snapToGroundDistance,omitempty"`
	ApplyImpulsesToDynamicBodies *bool    `json:"applyImpulsesToDynamicBodies,omitempty"`
}

// Tween animates one numeric render property (Pixi core has no tween
// system; the runtime owns this). Repeat -1 loops forever.
type Tween struct {
	Property   string  `json:"property"`
	From       float64 `json:"from"`
	To         float64 `json:"to"`
	DurationMS int     `json:"durationMs"`
	DelayMS    int     `json:"delayMs,omitempty"`
	Repeat     int     `json:"repeat,omitempty"`
	Yoyo       bool    `json:"yoyo,omitempty"`
	Easing     string  `json:"easing,omitempty"`
}

// Behavior declares autonomous motion for an entity, interpreted by the
// platform runtime's tick loop (the algorithmic completion layer). It is
// data, not code: fall = move down at Speed logical px/s, wrapping to the
// top (a falling pickup that is collected respawns at the top — falling
// pickups are renewable); chase = move toward the player at Speed px/s;
// patrol = oscillate horizontally around the authored position
// (Range px amplitude, Speed = angular speed); oscillate = the same
// vertically. A nil Behavior means the entity is static.
type Behavior struct {
	Type  string   `json:"type"`
	Speed *float64 `json:"speed,omitempty"`
	Range *float64 `json:"range,omitempty"`
}

type InputBinding struct {
	Action string `json:"action"`
	Input  string `json:"input"`
	Key    string `json:"key,omitempty"`
}

type Rule struct {
	ID         string      `json:"id"`
	When       Condition   `json:"when"`
	Conditions []Predicate `json:"conditions,omitempty"`
	Actions    []Action    `json:"actions"`
}

type Condition struct {
	Event       string   `json:"event"`
	Source      string   `json:"source,omitempty"`
	Target      string   `json:"target,omitempty"`
	InputAction string   `json:"inputAction,omitempty"`
	Value       *float64 `json:"value,omitempty"`
}

type Predicate struct {
	Variable string `json:"variable"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type Action struct {
	Type       string `json:"type"`
	Target     string `json:"target,omitempty"`
	Variable   string `json:"variable,omitempty"`
	Value      any    `json:"value,omitempty"`
	AssetID    string `json:"assetId,omitempty"`
	SceneID    string `json:"sceneId,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`
}

type Metadata struct {
	CreatorDisplayName string   `json:"creatorDisplayName,omitempty"`
	ParentGameID       string   `json:"parentGameId,omitempty"`
	ParentVersion      int      `json:"parentVersion,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

var (
	assetKinds     = enum("image", "audio", "video", "tilemap", "font", "data")
	assetSources   = enum("user_upload", "user_drawing", "generated", "licensed_library", "platform")
	entityKinds    = enum("player", "enemy", "obstacle", "pickup", "decoration", "trigger", "text")
	componentKinds = enum("movement", "health", "score", "collider", "collectible", "timer", "animation", "follow_path", "text_label", "physics_body", "character_controller", "particles", "tween", "filter")
	inputKinds     = enum("tap", "drag", "swipe", "virtual_joystick", "button", "tilt", "keyboard")
	eventKinds     = enum("game_start", "input", "collision", "collected", "timer_elapsed", "score_reached", "health_changed", "scene_entered", "contact_started", "contact_stopped", "contact_force", "entity_destroyed", "variable_changed", "region_entered", "region_exited", "animation_complete")
	actionKinds    = enum("add_score", "set_variable", "move_entity", "destroy_entity", "spawn_entity", "play_sound", "show_text", "change_scene", "set_health", "end_game", "change_health", "apply_impulse", "set_velocity", "set_angular_velocity", "add_force", "set_gravity", "teleport_entity", "set_alpha", "set_tint", "set_visible", "set_asset", "emit_particles", "camera_shake", "camera_follow")
	operators      = enum("eq", "neq", "gt", "gte", "lt", "lte")

	behaviorKinds     = enum("fall", "chase", "patrol", "oscillate", "spin", "pulse", "drift", "orbit")
	capabilityKinds   = enum("render.sprite", "render.animation", "render.shape", "render.text", "render.filters", "render.particles", "render.parallax", "physics.bodies", "physics.colliders", "physics.joints", "physics.character_controller", "physics.queries", "fx.tween", "fx.camera", "fx.emitters", "audio", "input.keyboard", "input.pointer", "shell.v1")
	blendModes        = enum("normal", "add", "multiply", "screen", "overlay", "darken", "lighten", "color-dodge", "color-burn", "hard-light", "soft-light", "difference", "exclusion", "hue", "saturation", "color", "luminosity", "none", "subtract", "erase", "linear-burn", "linear-dodge", "vivid-light", "linear-light", "pin-light", "hard-mix")
	physicsBodyKinds  = enum("dynamic", "fixed", "kinematic_position", "kinematic_velocity")
	physicsShapeKinds = enum("cuboid", "ball", "capsule", "convex", "segment")
	combineRules      = enum("average", "min", "multiply", "max", "geometric_mean", "clamped_sum")
	renderShapeKinds  = enum("rect", "rounded_rect", "circle", "ellipse", "polygon", "line")
	filterKinds       = enum("blur", "alpha", "color_matrix", "noise", "displacement")
	colorMatrixOps    = enum("brightness", "contrast", "saturate", "hue", "grayscale", "invert", "sepia", "black_and_white", "negative", "tint")
	jointKinds        = enum("fixed", "revolute", "prismatic", "rope", "spring")
	jointMotorModes   = enum("velocity", "position")
	tweenProperties   = enum("alpha", "scale", "scale_x", "scale_y", "rotation", "offset_x", "offset_y")
	tweenEasings      = enum("linear", "ease_in", "ease_out", "ease_in_out", "ease_out_back", "sine")
)

func enum(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
func has(values map[string]struct{}, value string) bool { _, ok := values[value]; return ok }

// DecodeAndValidate strictly decodes one JSON value and validates cross-references,
// bounded resource use and runtime capability names before a package is accepted.
func DecodeAndValidate(data []byte) (*Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var doc Document
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode Game IR: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode Game IR: trailing JSON value")
		}
		return nil, fmt.Errorf("decode Game IR: %w", err)
	}
	if err := Validate(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func Validate(d *Document) error {
	if d == nil {
		return fmt.Errorf("Game IR is nil")
	}
	if !has(supportedVersions, d.IRVersion) {
		return fmt.Errorf("unsupported irVersion %q (supported: %s, %s, %s)", d.IRVersion, Version, Version11, Version12)
	}
	if len(d.Capabilities) > MaxCapabilities {
		return fmt.Errorf("capability limit exceeded: %d > %d", len(d.Capabilities), MaxCapabilities)
	}
	{
		seen := enum()
		for _, c := range d.Capabilities {
			if !has(capabilityKinds, c) {
				return fmt.Errorf("unsupported capability %q", c)
			}
			if has(seen, c) {
				return fmt.Errorf("duplicate capability %q", c)
			}
			seen[c] = struct{}{}
		}
	}
	if !validID(d.Game.ID) || strings.TrimSpace(d.Game.Title) == "" {
		return fmt.Errorf("game id and title are required")
	}
	if !has(enum("portrait", "landscape", "any"), d.Game.Orientation) {
		return fmt.Errorf("unsupported orientation %q", d.Game.Orientation)
	}
	if d.Game.SessionTargetSeconds != 0 && (d.Game.SessionTargetSeconds < 5 || d.Game.SessionTargetSeconds > 3600) {
		return fmt.Errorf("sessionTargetSeconds must be between 5 and 3600")
	}
	if d.Game.Audience != "" && !has(enum("kids", "family", "teens", "adults", "general"), d.Game.Audience) {
		return fmt.Errorf("unsupported audience %q", d.Game.Audience)
	}
	if len(d.Assets) > MaxAssets {
		return fmt.Errorf("asset limit exceeded: %d > %d", len(d.Assets), MaxAssets)
	}
	if len(d.Scenes) == 0 || len(d.Scenes) > MaxScenes {
		return fmt.Errorf("scene count must be between 1 and %d", MaxScenes)
	}
	if len(d.Rules) > MaxRules {
		return fmt.Errorf("rule limit exceeded: %d > %d", len(d.Rules), MaxRules)
	}
	if len(d.InputBindings) > MaxInputBindings {
		return fmt.Errorf("input binding limit exceeded: %d > %d", len(d.InputBindings), MaxInputBindings)
	}
	if len(d.Metadata.Tags) > 20 {
		return fmt.Errorf("metadata tag limit exceeded")
	}
	if d.Metadata.ParentVersion < 0 {
		return fmt.Errorf("parentVersion cannot be negative")
	}

	assets := make(map[string]struct{}, len(d.Assets))
	for i, a := range d.Assets {
		if !validID(a.ID) {
			return fmt.Errorf("assets[%d].id is invalid", i)
		}
		if _, exists := assets[a.ID]; exists {
			return fmt.Errorf("duplicate asset id %q", a.ID)
		}
		if !has(assetKinds, a.Kind) {
			return fmt.Errorf("asset %q has unsupported kind %q", a.ID, a.Kind)
		}
		if err := validatePackagePath(a.URI); err != nil {
			return fmt.Errorf("asset %q: %w", a.ID, err)
		}
		if a.Source != "" && !has(assetSources, a.Source) {
			return fmt.Errorf("asset %q has unsupported source %q", a.ID, a.Source)
		}
		if a.SHA256 != "" && (len(a.SHA256) != 64 || !isHex(a.SHA256)) {
			return fmt.Errorf("asset %q has invalid sha256", a.ID)
		}
		assets[a.ID] = struct{}{}
	}

	scenes := make(map[string]struct{}, len(d.Scenes))
	entities := make(map[string]struct{})
	templateIDs := enum()
	totalEntities := 0
	for si, scene := range d.Scenes {
		if !validID(scene.ID) {
			return fmt.Errorf("scenes[%d].id is invalid", si)
		}
		if _, exists := scenes[scene.ID]; exists {
			return fmt.Errorf("duplicate scene id %q", scene.ID)
		}
		if scene.BackgroundAssetID != "" && !has(assets, scene.BackgroundAssetID) {
			return fmt.Errorf("scene %q references missing background asset %q", scene.ID, scene.BackgroundAssetID)
		}
		if scene.BackgroundColor != "" && !validColor(scene.BackgroundColor) {
			return fmt.Errorf("scene %q has invalid backgroundColor %q", scene.ID, scene.BackgroundColor)
		}
		if err := validateVec2(scene.Gravity, 100000); err != nil {
			return fmt.Errorf("scene %q gravity: %w", scene.ID, err)
		}
		if len(scene.WorldSize) != 0 && (len(scene.WorldSize) != 2 || !finite(scene.WorldSize[0]) || !finite(scene.WorldSize[1]) || scene.WorldSize[0] <= 0 || scene.WorldSize[1] <= 0 || scene.WorldSize[0] > 100000 || scene.WorldSize[1] > 100000) {
			return fmt.Errorf("scene %q worldSize must contain two positive bounded numbers", scene.ID)
		}
		if scene.PixelsPerMeter != nil && (*scene.PixelsPerMeter < 10 || *scene.PixelsPerMeter > 1000) {
			return fmt.Errorf("scene %q pixelsPerMeter must be between 10 and 1000", scene.ID)
		}
		layerIDs := enum()
		for _, layer := range scene.Layers {
			if !validID(layer.ID) || has(layerIDs, layer.ID) {
				return fmt.Errorf("scene %q has invalid or duplicate layer id %q", scene.ID, layer.ID)
			}
			layerIDs[layer.ID] = struct{}{}
			if layer.AssetID != "" && !has(assets, layer.AssetID) {
				return fmt.Errorf("scene %q layer %q references missing asset %q", scene.ID, layer.ID, layer.AssetID)
			}
			if err := validateVec2(layer.Parallax, 10); err != nil {
				return fmt.Errorf("scene %q layer %q parallax: %w", scene.ID, layer.ID, err)
			}
			if layer.Tint != "" && !validColor(layer.Tint) {
				return fmt.Errorf("scene %q layer %q has invalid tint %q", scene.ID, layer.ID, layer.Tint)
			}
			if layer.Alpha != nil && (*layer.Alpha < 0 || *layer.Alpha > 1) {
				return fmt.Errorf("scene %q layer %q alpha must be between 0 and 1", scene.ID, layer.ID)
			}
		}
		for ei, emitter := range scene.Emitters {
			if err := validateEmitter(emitter, assets); err != nil {
				return fmt.Errorf("scene %q emitters[%d]: %w", scene.ID, ei, err)
			}
		}
		if len(scene.Entities) > MaxEntities {
			return fmt.Errorf("scene %q entity limit exceeded", scene.ID)
		}
		if len(scene.Joints) > MaxJoints {
			return fmt.Errorf("scene %q joint limit exceeded", scene.ID)
		}
		totalEntities += len(scene.Entities) + len(scene.Templates)
		sceneEntities := enum()
		for _, entity := range scene.Entities {
			if err := validateEntity(entity, assets, entities); err != nil {
				return err
			}
			sceneEntities[entity.ID] = struct{}{}
		}
		for _, tpl := range scene.Templates {
			if err := validateEntity(tpl, assets, entities); err != nil {
				return err
			}
			templateIDs[tpl.ID] = struct{}{}
		}
		jointIDs := enum()
		for _, j := range scene.Joints {
			if !validID(j.ID) || has(jointIDs, j.ID) {
				return fmt.Errorf("scene %q has invalid or duplicate joint id %q", scene.ID, j.ID)
			}
			jointIDs[j.ID] = struct{}{}
			if !has(jointKinds, j.Type) {
				return fmt.Errorf("joint %q has unsupported type %q", j.ID, j.Type)
			}
			if !has(sceneEntities, j.BodyA) || !has(sceneEntities, j.BodyB) {
				return fmt.Errorf("joint %q references a body outside scene %q", j.ID, scene.ID)
			}
			if j.BodyA == j.BodyB {
				return fmt.Errorf("joint %q connects body %q to itself", j.ID, j.BodyA)
			}
			if err := validateVec2(j.AnchorA, 100000); err != nil {
				return fmt.Errorf("joint %q anchorA: %w", j.ID, err)
			}
			if err := validateVec2(j.AnchorB, 100000); err != nil {
				return fmt.Errorf("joint %q anchorB: %w", j.ID, err)
			}
			if len(j.Axis) != 0 && (len(j.Axis) != 2 || !finite(j.Axis[0]) || !finite(j.Axis[1]) || (j.Axis[0] == 0 && j.Axis[1] == 0)) {
				return fmt.Errorf("joint %q axis must be a non-zero finite pair", j.ID)
			}
			if len(j.Limits) != 0 && (len(j.Limits) != 2 || !finite(j.Limits[0]) || !finite(j.Limits[1]) || j.Limits[0] > j.Limits[1]) {
				return fmt.Errorf("joint %q limits must be an ordered finite pair", j.ID)
			}
			for label, v := range map[string]*float64{"length": j.Length, "restLength": j.RestLength, "stiffness": j.Stiffness, "damping": j.Damping} {
				if v != nil && (!finite(*v) || *v < 0 || *v > 1000000) {
					return fmt.Errorf("joint %q %s out of range", j.ID, label)
				}
			}
			if j.Motor != nil {
				if !has(jointMotorModes, j.Motor.Mode) {
					return fmt.Errorf("joint %q has unsupported motor mode %q", j.ID, j.Motor.Mode)
				}
				if !finite(j.Motor.Target) || abs(j.Motor.Target) > 1000000 {
					return fmt.Errorf("joint %q motor target out of range", j.ID)
				}
				if j.Motor.MaxForce != nil && (*j.Motor.MaxForce <= 0 || *j.Motor.MaxForce > 1000000) {
					return fmt.Errorf("joint %q motor maxForce out of range", j.ID)
				}
				for label, v := range map[string]*float64{"stiffness": j.Motor.Stiffness, "damping": j.Motor.Damping} {
					if v != nil && (!finite(*v) || *v < 0 || *v > 1000000) {
						return fmt.Errorf("joint %q motor %s out of range", j.ID, label)
					}
				}
			}
		}
		scenes[scene.ID] = struct{}{}
	}
	if totalEntities > MaxTotalEntities {
		return fmt.Errorf("total entity limit exceeded: %d > %d", totalEntities, MaxTotalEntities)
	}
	if !has(scenes, d.Game.EntryScene) {
		return fmt.Errorf("entryScene %q does not exist", d.Game.EntryScene)
	}

	bindings := enum()
	actionsByName := enum()
	for i, b := range d.InputBindings {
		if !validID(b.Action) {
			return fmt.Errorf("inputBindings[%d].action is invalid", i)
		}
		if !has(inputKinds, b.Input) {
			return fmt.Errorf("input binding %q has unsupported input %q", b.Action, b.Input)
		}
		if len(b.Key) > 64 {
			return fmt.Errorf("input binding %q key too long", b.Action)
		}
		key := b.Action + "\x00" + b.Input
		if has(bindings, key) {
			return fmt.Errorf("duplicate input binding %q/%q", b.Action, b.Input)
		}
		bindings[key] = struct{}{}
		actionsByName[b.Action] = struct{}{}
	}

	ruleIDs := enum()
	for _, rule := range d.Rules {
		if !validID(rule.ID) || has(ruleIDs, rule.ID) {
			return fmt.Errorf("invalid or duplicate rule id %q", rule.ID)
		}
		ruleIDs[rule.ID] = struct{}{}
		if !has(eventKinds, rule.When.Event) {
			return fmt.Errorf("rule %q has unsupported event %q", rule.ID, rule.When.Event)
		}
		for label, id := range map[string]string{"source": rule.When.Source, "target": rule.When.Target} {
			if id != "" && !has(entities, id) {
				return fmt.Errorf("rule %q references missing %s entity %q", rule.ID, label, id)
			}
		}
		if rule.When.InputAction != "" && !has(actionsByName, rule.When.InputAction) {
			return fmt.Errorf("rule %q references unbound input action %q", rule.ID, rule.When.InputAction)
		}
		if len(rule.Conditions) > 8 {
			return fmt.Errorf("rule %q condition limit exceeded", rule.ID)
		}
		for _, p := range rule.Conditions {
			if strings.TrimSpace(p.Variable) == "" || !has(operators, p.Operator) || p.Value == nil {
				return fmt.Errorf("rule %q has invalid condition", rule.ID)
			}
		}
		if len(rule.Actions) == 0 || len(rule.Actions) > 16 {
			return fmt.Errorf("rule %q must have 1 to 16 actions", rule.ID)
		}
		for _, action := range rule.Actions {
			if !has(actionKinds, action.Type) {
				return fmt.Errorf("rule %q has unsupported action %q", rule.ID, action.Type)
			}
			if action.Target != "" && !has(entities, action.Target) {
				return fmt.Errorf("rule %q action references missing entity %q", rule.ID, action.Target)
			}
			if action.AssetID != "" && !has(assets, action.AssetID) {
				return fmt.Errorf("rule %q action references missing asset %q", rule.ID, action.AssetID)
			}
			if action.SceneID != "" && !has(scenes, action.SceneID) {
				return fmt.Errorf("rule %q action references missing scene %q", rule.ID, action.SceneID)
			}
			if action.DurationMS < 0 || action.DurationMS > 60000 {
				return fmt.Errorf("rule %q action duration out of range", rule.ID)
			}
			switch action.Type {
			case "apply_impulse", "set_velocity", "add_force", "set_gravity", "teleport_entity":
				if err := validateVectorValue(action.Value); err != nil {
					return fmt.Errorf("rule %q action %q: %w", rule.ID, action.Type, err)
				}
				if action.Type != "set_gravity" && action.Target == "" {
					return fmt.Errorf("rule %q action %q requires target", rule.ID, action.Type)
				}
			case "set_angular_velocity":
				v, ok := asNumber(action.Value)
				if !ok || !finite(v) || abs(v) > 1000 || action.Target == "" {
					return fmt.Errorf("rule %q set_angular_velocity requires target and a bounded numeric value", rule.ID)
				}
			case "change_health":
				v, ok := asNumber(action.Value)
				if !ok || !finite(v) || abs(v) > 1000 {
					return fmt.Errorf("rule %q change_health requires a bounded numeric value", rule.ID)
				}
			case "camera_shake":
				v, ok := asNumber(action.Value)
				if !ok || !finite(v) || v < 0 || v > 100 {
					return fmt.Errorf("rule %q camera_shake requires a value between 0 and 100", rule.ID)
				}
			case "set_alpha":
				v, ok := asNumber(action.Value)
				if !ok || v < 0 || v > 1 || action.Target == "" {
					return fmt.Errorf("rule %q set_alpha requires target and a value between 0 and 1", rule.ID)
				}
			case "set_tint":
				s, ok := action.Value.(string)
				if !ok || !validColor(s) || action.Target == "" {
					return fmt.Errorf("rule %q set_tint requires target and a hex color value", rule.ID)
				}
			case "set_visible":
				if _, ok := action.Value.(bool); !ok || action.Target == "" {
					return fmt.Errorf("rule %q set_visible requires target and a boolean value", rule.ID)
				}
			case "set_asset":
				if action.Target == "" || action.AssetID == "" {
					return fmt.Errorf("rule %q set_asset requires target and assetId", rule.ID)
				}
			case "emit_particles":
				if action.Value != nil {
					v, ok := asNumber(action.Value)
					if !ok || v < 1 || v > 5000 {
						return fmt.Errorf("rule %q emit_particles count must be between 1 and 5000", rule.ID)
					}
				}
			case "spawn_entity":
				if !has(templateIDs, action.Target) {
					return fmt.Errorf("rule %q spawn_entity target %q is not a scene template", rule.ID, action.Target)
				}
			case "camera_follow":
				if action.Target == "" {
					return fmt.Errorf("rule %q camera_follow requires target", rule.ID)
				}
			}
			if action.Type == "set_variable" && strings.TrimSpace(action.Variable) == "" {
				return fmt.Errorf("rule %q set_variable requires variable", rule.ID)
			}
			if action.Type == "change_scene" && action.SceneID == "" {
				return fmt.Errorf("rule %q change_scene requires sceneId", rule.ID)
			}
			if action.Type == "play_sound" && action.AssetID == "" {
				return fmt.Errorf("rule %q play_sound requires assetId", rule.ID)
			}
		}
	}
	if err := validateExperience(d); err != nil {
		return err
	}
	return nil
}

// validateEntity checks one entity (scene entity or spawn template)
// and registers its ID in the shared entities set.
func validateEntity(entity Entity, assets map[string]struct{}, entities map[string]struct{}) error {
	if !validID(entity.ID) {
		return fmt.Errorf("entity id %q is invalid", entity.ID)
	}
	if _, exists := entities[entity.ID]; exists {
		return fmt.Errorf("duplicate entity id %q", entity.ID)
	}
	if !has(entityKinds, entity.Kind) {
		return fmt.Errorf("entity %q has unsupported kind %q", entity.ID, entity.Kind)
	}
	if entity.AssetID != "" && !has(assets, entity.AssetID) {
		return fmt.Errorf("entity %q references missing asset %q", entity.ID, entity.AssetID)
	}
	if len(entity.Position) != 2 || !finite(entity.Position[0]) || !finite(entity.Position[1]) || abs(entity.Position[0]) > 100000 || abs(entity.Position[1]) > 100000 {
		return fmt.Errorf("entity %q position must contain two finite bounded numbers", entity.ID)
	}
	if len(entity.Size) != 0 && (len(entity.Size) != 2 || !finite(entity.Size[0]) || !finite(entity.Size[1]) || entity.Size[0] <= 0 || entity.Size[1] <= 0 || entity.Size[0] > 100000 || entity.Size[1] > 100000) {
		return fmt.Errorf("entity %q size must contain two positive bounded numbers", entity.ID)
	}
	if !finite(entity.Rotation) || abs(entity.Rotation) > 360000 {
		return fmt.Errorf("entity %q rotation out of range", entity.ID)
	}
	if entity.ZIndex < -10000 || entity.ZIndex > 10000 {
		return fmt.Errorf("entity %q zIndex out of range", entity.ID)
	}
	if len(entity.Components) > 16 {
		return fmt.Errorf("entity %q component limit exceeded", entity.ID)
	}
	if entity.Behavior != nil {
		b := entity.Behavior
		if !has(behaviorKinds, b.Type) {
			return fmt.Errorf("entity %q has unsupported behavior type %q", entity.ID, b.Type)
		}
		if b.Speed != nil && (!finite(*b.Speed) || *b.Speed < 0 || *b.Speed > 100000) {
			return fmt.Errorf("entity %q behavior speed out of range", entity.ID)
		}
		if b.Range != nil && (!finite(*b.Range) || *b.Range < 0 || *b.Range > 100000) {
			return fmt.Errorf("entity %q behavior range out of range", entity.ID)
		}
	}
	seen := enum()
	for _, component := range entity.Components {
		if !has(componentKinds, component) {
			return fmt.Errorf("entity %q has unsupported component %q", entity.ID, component)
		}
		if has(seen, component) {
			return fmt.Errorf("entity %q has duplicate component %q", entity.ID, component)
		}
		seen[component] = struct{}{}
	}
	if err := validateRender(entity, assets); err != nil {
		return err
	}
	if err := validatePhysics(entity); err != nil {
		return err
	}
	if c := entity.Character; c != nil {
		for label, v := range map[string]*float64{"offset": c.Offset, "autostepMaxHeight": c.AutostepMaxHeight, "autostepMinWidth": c.AutostepMinWidth, "snapToGroundDistance": c.SnapToGroundDistance} {
			if v != nil && (!finite(*v) || *v < 0 || *v > 10000) {
				return fmt.Errorf("entity %q character %s out of range", entity.ID, label)
			}
		}
		for label, v := range map[string]*float64{"maxSlopeClimbDegrees": c.MaxSlopeClimbDegrees, "minSlopeSlideDegrees": c.MinSlopeSlideDegrees} {
			if v != nil && (!finite(*v) || *v < 0 || *v > 90) {
				return fmt.Errorf("entity %q character %s must be between 0 and 90", entity.ID, label)
			}
		}
	}
	if len(entity.Tweens) > 8 {
		return fmt.Errorf("entity %q tween limit exceeded", entity.ID)
	}
	for _, tw := range entity.Tweens {
		if !has(tweenProperties, tw.Property) {
			return fmt.Errorf("entity %q has unsupported tween property %q", entity.ID, tw.Property)
		}
		if !finite(tw.From) || !finite(tw.To) || abs(tw.From) > 1000000 || abs(tw.To) > 1000000 {
			return fmt.Errorf("entity %q tween from/to out of range", entity.ID)
		}
		if tw.DurationMS < 10 || tw.DurationMS > 600000 {
			return fmt.Errorf("entity %q tween durationMs out of range", entity.ID)
		}
		if tw.DelayMS < 0 || tw.DelayMS > 60000 {
			return fmt.Errorf("entity %q tween delayMs out of range", entity.ID)
		}
		if tw.Repeat < -1 || tw.Repeat > 10000 {
			return fmt.Errorf("entity %q tween repeat out of range", entity.ID)
		}
		if tw.Easing != "" && !has(tweenEasings, tw.Easing) {
			return fmt.Errorf("entity %q has unsupported tween easing %q", entity.ID, tw.Easing)
		}
	}
	entities[entity.ID] = struct{}{}
	return nil
}

func validateRender(entity Entity, assets map[string]struct{}) error {
	r := entity.Render
	if r == nil {
		return nil
	}
	id := entity.ID
	if r.Tint != "" && !validColor(r.Tint) {
		return fmt.Errorf("entity %q render tint %q is not a hex color", id, r.Tint)
	}
	if r.Alpha != nil && (*r.Alpha < 0 || *r.Alpha > 1) {
		return fmt.Errorf("entity %q render alpha must be between 0 and 1", id)
	}
	if r.BlendMode != "" && !has(blendModes, r.BlendMode) {
		return fmt.Errorf("entity %q has unsupported blendMode %q", id, r.BlendMode)
	}
	if len(r.Anchor) != 0 && (len(r.Anchor) != 2 || !finite(r.Anchor[0]) || !finite(r.Anchor[1]) || r.Anchor[0] < -1 || r.Anchor[0] > 2 || r.Anchor[1] < -1 || r.Anchor[1] > 2) {
		return fmt.Errorf("entity %q render anchor must be a pair within [-1, 2]", id)
	}
	if len(r.Scale) != 0 && (len(r.Scale) != 2 || !finite(r.Scale[0]) || !finite(r.Scale[1]) || r.Scale[0] == 0 || r.Scale[1] == 0 || abs(r.Scale[0]) > 100 || abs(r.Scale[1]) > 100) {
		return fmt.Errorf("entity %q render scale must be a non-zero bounded pair", id)
	}
	if a := r.Animation; a != nil {
		if len(a.Frames) == 0 || len(a.Frames) > 256 {
			return fmt.Errorf("entity %q animation frames must list 1 to 256 assets", id)
		}
		for _, f := range a.Frames {
			if !has(assets, f) {
				return fmt.Errorf("entity %q animation references missing asset %q", id, f)
			}
		}
		if a.FPS != nil && (!finite(*a.FPS) || *a.FPS <= 0 || *a.FPS > 240) {
			return fmt.Errorf("entity %q animation fps out of range", id)
		}
	}
	if s := r.Shape; s != nil {
		if !has(renderShapeKinds, s.Type) {
			return fmt.Errorf("entity %q has unsupported shape type %q", id, s.Type)
		}
		if s.Fill != "" && !validColor(s.Fill) {
			return fmt.Errorf("entity %q shape fill %q is not a hex color", id, s.Fill)
		}
		if s.Stroke != "" && !validColor(s.Stroke) {
			return fmt.Errorf("entity %q shape stroke %q is not a hex color", id, s.Stroke)
		}
		if s.StrokeWidth != nil && (*s.StrokeWidth < 0 || *s.StrokeWidth > 1000) {
			return fmt.Errorf("entity %q shape strokeWidth out of range", id)
		}
		if s.CornerRadius != nil && (*s.CornerRadius < 0 || *s.CornerRadius > 10000) {
			return fmt.Errorf("entity %q shape cornerRadius out of range", id)
		}
		if len(s.Points) > 64 {
			return fmt.Errorf("entity %q shape point limit exceeded", id)
		}
		for _, p := range s.Points {
			if len(p) != 2 || !finite(p[0]) || !finite(p[1]) || abs(p[0]) > 100000 || abs(p[1]) > 100000 {
				return fmt.Errorf("entity %q shape points must be bounded pairs", id)
			}
		}
		if (s.Type == "polygon" || s.Type == "line") && len(s.Points) < 2 {
			return fmt.Errorf("entity %q shape %q requires at least 2 points", id, s.Type)
		}
	}
	if t := r.Text; t != nil {
		if len(t.Content) > 2000 {
			return fmt.Errorf("entity %q text content too long", id)
		}
		if t.FontSize != nil && (*t.FontSize < 1 || *t.FontSize > 500) {
			return fmt.Errorf("entity %q text fontSize out of range", id)
		}
		if t.Fill != "" && !validColor(t.Fill) {
			return fmt.Errorf("entity %q text fill %q is not a hex color", id, t.Fill)
		}
		if t.Stroke != "" && !validColor(t.Stroke) {
			return fmt.Errorf("entity %q text stroke %q is not a hex color", id, t.Stroke)
		}
		if t.Align != "" && !has(enum("left", "center", "right"), t.Align) {
			return fmt.Errorf("entity %q text align %q unsupported", id, t.Align)
		}
		if len(t.FontWeight) > 20 {
			return fmt.Errorf("entity %q text fontWeight too long", id)
		}
	}
	if len(r.Filters) > 8 {
		return fmt.Errorf("entity %q filter limit exceeded", id)
	}
	for _, f := range r.Filters {
		if !has(filterKinds, f.Type) {
			return fmt.Errorf("entity %q has unsupported filter %q", id, f.Type)
		}
		if f.Value != nil && (!finite(*f.Value) || abs(*f.Value) > 10000) {
			return fmt.Errorf("entity %q filter value out of range", id)
		}
		if f.Type == "color_matrix" && f.Operation != "" && !has(colorMatrixOps, f.Operation) {
			return fmt.Errorf("entity %q has unsupported color_matrix operation %q", id, f.Operation)
		}
		if f.Type == "displacement" && f.AssetID != "" && !has(assets, f.AssetID) {
			return fmt.Errorf("entity %q displacement filter references missing asset %q", id, f.AssetID)
		}
		if f.Scale != nil && (*f.Scale < 0 || *f.Scale > 10000) {
			return fmt.Errorf("entity %q filter scale out of range", id)
		}
	}
	if r.Particles != nil {
		if err := validateEmitter(*r.Particles, assets); err != nil {
			return fmt.Errorf("entity %q particles: %w", id, err)
		}
	}
	return nil
}

func validatePhysics(entity Entity) error {
	p := entity.Physics
	if p == nil {
		return nil
	}
	id := entity.ID
	if !has(physicsBodyKinds, p.Body) {
		return fmt.Errorf("entity %q has unsupported physics body %q", id, p.Body)
	}
	if p.Shape != "" && !has(physicsShapeKinds, p.Shape) {
		return fmt.Errorf("entity %q has unsupported physics shape %q", id, p.Shape)
	}
	if len(p.Points) > 64 {
		return fmt.Errorf("entity %q physics point limit exceeded", id)
	}
	for _, pt := range p.Points {
		if len(pt) != 2 || !finite(pt[0]) || !finite(pt[1]) || abs(pt[0]) > 100000 || abs(pt[1]) > 100000 {
			return fmt.Errorf("entity %q physics points must be bounded pairs", id)
		}
	}
	switch p.Shape {
	case "convex":
		if len(p.Points) < 3 {
			return fmt.Errorf("entity %q convex physics shape requires at least 3 points", id)
		}
	case "segment":
		if len(p.Points) != 2 {
			return fmt.Errorf("entity %q segment physics shape requires exactly 2 points", id)
		}
	}
	if p.Density != nil && (!finite(*p.Density) || *p.Density < 0 || *p.Density > 10000) {
		return fmt.Errorf("entity %q physics density out of range", id)
	}
	if p.Friction != nil && (!finite(*p.Friction) || *p.Friction < 0 || *p.Friction > 100) {
		return fmt.Errorf("entity %q physics friction out of range", id)
	}
	if p.Restitution != nil && (!finite(*p.Restitution) || *p.Restitution < 0 || *p.Restitution > 10) {
		return fmt.Errorf("entity %q physics restitution out of range", id)
	}
	if p.FrictionCombineRule != "" && !has(combineRules, p.FrictionCombineRule) {
		return fmt.Errorf("entity %q has unsupported frictionCombineRule %q", id, p.FrictionCombineRule)
	}
	if p.RestitutionCombineRule != "" && !has(combineRules, p.RestitutionCombineRule) {
		return fmt.Errorf("entity %q has unsupported restitutionCombineRule %q", id, p.RestitutionCombineRule)
	}
	if p.GravityScale != nil && (!finite(*p.GravityScale) || *p.GravityScale < -100 || *p.GravityScale > 100) {
		return fmt.Errorf("entity %q physics gravityScale out of range", id)
	}
	if p.LinearDamping != nil && (!finite(*p.LinearDamping) || *p.LinearDamping < 0 || *p.LinearDamping > 1000) {
		return fmt.Errorf("entity %q physics linearDamping out of range", id)
	}
	if p.AngularDamping != nil && (!finite(*p.AngularDamping) || *p.AngularDamping < 0 || *p.AngularDamping > 1000) {
		return fmt.Errorf("entity %q physics angularDamping out of range", id)
	}
	if p.AngularVelocity != nil && (!finite(*p.AngularVelocity) || abs(*p.AngularVelocity) > 1000) {
		return fmt.Errorf("entity %q physics angularVelocity out of range", id)
	}
	if err := validateVec2(p.InitialVelocity, 1000000); err != nil {
		return fmt.Errorf("entity %q physics initialVelocity: %w", id, err)
	}
	if len(p.LockTranslation) != 0 && len(p.LockTranslation) != 2 {
		return fmt.Errorf("entity %q physics lockTranslation must be a pair", id)
	}
	if g := p.CollisionGroups; g != nil {
		for label, groups := range map[string][]int{"memberships": g.Memberships, "filter": g.Filter} {
			if len(groups) > 16 {
				return fmt.Errorf("entity %q collisionGroups %s limit exceeded", id, label)
			}
			for _, n := range groups {
				if n < 0 || n > 15 {
					return fmt.Errorf("entity %q collisionGroups %s entry %d out of range 0..15", id, label, n)
				}
			}
		}
	}
	if p.Dominance != nil && (*p.Dominance < -127 || *p.Dominance > 127) {
		return fmt.Errorf("entity %q physics dominance must be between -127 and 127", id)
	}
	return nil
}

func validateEmitter(e ParticleEmitter, assets map[string]struct{}) error {
	if e.TextureAssetID != "" && !has(assets, e.TextureAssetID) {
		return fmt.Errorf("references missing texture asset %q", e.TextureAssetID)
	}
	for label, r := range map[string][]float64{"lifetime": e.Lifetime, "speed": e.Speed} {
		if len(r) != 0 && (len(r) != 2 || !finite(r[0]) || !finite(r[1]) || r[0] < 0 || r[1] < r[0] || r[1] > 100000) {
			return fmt.Errorf("%s must be an ordered [min, max] range within bounds", label)
		}
	}
	for label, v := range map[string]*float64{"rate": e.Rate, "burst": e.Burst, "direction": e.Direction, "gravity": e.Gravity, "startScale": e.StartScale, "endScale": e.EndScale} {
		if v != nil && (!finite(*v) || abs(*v) > 100000) {
			return fmt.Errorf("%s out of range", label)
		}
	}
	if e.Rate != nil && *e.Rate < 0 {
		return fmt.Errorf("rate cannot be negative")
	}
	if e.Burst != nil && *e.Burst < 0 {
		return fmt.Errorf("burst cannot be negative")
	}
	if e.Spread != nil && (!finite(*e.Spread) || *e.Spread < 0 || *e.Spread > 360) {
		return fmt.Errorf("spread must be between 0 and 360")
	}
	for label, v := range map[string]*float64{"startAlpha": e.StartAlpha, "endAlpha": e.EndAlpha} {
		if v != nil && (*v < 0 || *v > 1) {
			return fmt.Errorf("%s must be between 0 and 1", label)
		}
	}
	if e.Tint != "" && !validColor(e.Tint) {
		return fmt.Errorf("invalid tint %q", e.Tint)
	}
	if e.BlendMode != "" && !has(blendModes, e.BlendMode) {
		return fmt.Errorf("unsupported blendMode %q", e.BlendMode)
	}
	if e.MaxParticles < 0 || e.MaxParticles > 100000 {
		return fmt.Errorf("maxParticles out of range")
	}
	return nil
}

// validateVec2 checks an optional [x, y] field: absent is fine; a
// present value must be exactly two finite numbers within bound.
func validateVec2(v []float64, bound float64) error {
	if len(v) == 0 {
		return nil
	}
	if len(v) != 2 || !finite(v[0]) || !finite(v[1]) || abs(v[0]) > bound || abs(v[1]) > bound {
		return fmt.Errorf("must contain two finite numbers within ±%v", bound)
	}
	return nil
}

func validateVectorValue(v any) error {
	pair, ok := v.([]any)
	if !ok || len(pair) != 2 {
		return fmt.Errorf("value must be a [x, y] pair")
	}
	for _, c := range pair {
		n, ok := asNumber(c)
		if !ok || !finite(n) || abs(n) > 1000000 {
			return fmt.Errorf("value components must be bounded numbers")
		}
	}
	return nil
}

func asNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// validColor accepts CSS hex colors: #rgb, #rrggbb, #rrggbbaa.
func validColor(s string) bool {
	if len(s) != 4 && len(s) != 7 && len(s) != 9 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	return isHex(s[1:])
}

func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
func validatePackagePath(uri string) error {
	if strings.TrimSpace(uri) == "" || strings.Contains(uri, "\\") || strings.Contains(uri, ":") {
		return fmt.Errorf("URI must be a relative package path")
	}
	if strings.HasPrefix(uri, "/") || path.IsAbs(uri) || path.Clean(uri) != uri || uri == "." || uri == ".." || strings.HasPrefix(uri, "../") {
		return fmt.Errorf("URI %q is not a clean relative path", uri)
	}
	return nil
}
func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
func finite(v float64) bool {
	return v == v && v < 1.7976931348623157e308 && v > -1.7976931348623157e308
}
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
