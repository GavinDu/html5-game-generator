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

const (
	MaxAssets        = 256
	MaxScenes        = 64
	MaxEntities      = 500
	MaxTotalEntities = 2000
	MaxRules         = 256
	MaxInputBindings = 32
)

type Document struct {
	IRVersion     string         `json:"irVersion"`
	Game          Game           `json:"game"`
	Assets        []Asset        `json:"assets"`
	Scenes        []Scene        `json:"scenes"`
	InputBindings []InputBinding `json:"inputBindings"`
	Rules         []Rule         `json:"rules"`
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
	ID                string   `json:"id"`
	BackgroundAssetID string   `json:"backgroundAssetId,omitempty"`
	Entities          []Entity `json:"entities"`
}

type Entity struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	AssetID    string         `json:"assetId,omitempty"`
	Position   []float64      `json:"position"`
	Size       []float64      `json:"size,omitempty"`
	Components []string       `json:"components"`
	Properties map[string]any `json:"properties,omitempty"`
	Behavior   *Behavior      `json:"behavior,omitempty"`
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
	componentKinds = enum("movement", "health", "score", "collider", "collectible", "timer", "animation", "follow_path", "text_label")
	inputKinds     = enum("tap", "drag", "swipe", "virtual_joystick", "button", "tilt", "keyboard")
	eventKinds     = enum("game_start", "input", "collision", "collected", "timer_elapsed", "score_reached", "health_changed", "scene_entered")
	actionKinds    = enum("add_score", "set_variable", "move_entity", "destroy_entity", "spawn_entity", "play_sound", "show_text", "change_scene", "set_health", "end_game")
	operators      = enum("eq", "neq", "gt", "gte", "lt", "lte")
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
	if d.IRVersion != Version {
		return fmt.Errorf("unsupported irVersion %q (supported: %s)", d.IRVersion, Version)
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
		if len(scene.Entities) > MaxEntities {
			return fmt.Errorf("scene %q entity limit exceeded", scene.ID)
		}
		totalEntities += len(scene.Entities)
		for ei, entity := range scene.Entities {
			if !validID(entity.ID) {
				return fmt.Errorf("scene %q entity[%d] id is invalid", scene.ID, ei)
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
			if len(entity.Components) > 16 {
				return fmt.Errorf("entity %q component limit exceeded", entity.ID)
			}
			if entity.Behavior != nil {
				b := entity.Behavior
				if !has(enum("fall", "chase", "patrol", "oscillate"), b.Type) {
					return fmt.Errorf("entity %q has unsupported behavior type %q", entity.ID, b.Type)
				}
				if b.Speed != nil && (!finite(*b.Speed) || *b.Speed < 0 || *b.Speed > 1000) {
					return fmt.Errorf("entity %q behavior speed must be between 0 and 1000", entity.ID)
				}
				if b.Range != nil && (!finite(*b.Range) || *b.Range < 0 || *b.Range > 500) {
					return fmt.Errorf("entity %q behavior range must be between 0 and 500", entity.ID)
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
			entities[entity.ID] = struct{}{}
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
	return nil
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
