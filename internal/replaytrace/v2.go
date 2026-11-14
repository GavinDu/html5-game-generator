// Package replaytrace provides canonical, bounded data-only semantic input
// traces for a future independent deterministic replay interpreter. This
// library cannot authorize a run verdict or expand published IR capabilities.
package replaytrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"doodlequest/internal/gameir"
)

const (
	SchemaV2           = "semantic-input-trace-v2"
	MaxEvents          = 2000
	MaxDurationTicks   = 30 * 60 * 60 // 30 minutes at the canonical 60 Hz.
	MaxFixedCoordinate = 1000000000   // 1 million logical pixels at millipixel scale.
	maxActionBytes     = 128
)

// Position is a fixed-point (1/1000 logical pixel) position. No floats,
// display-frame delta time, or host-viewport-dependent raw coordinates.
type Position struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

// Event is one declared semantic action edge at a 60 Hz simulation tick.
// Ordinal starts at zero on each tick and increments without gaps.
type Event struct {
	Tick     uint32 `json:"tick"`
	Ordinal  uint32 `json:"ordinal"`
	Action   string `json:"action"`
	Device   string `json:"device"` // pointer or keyboard
	Phase    string `json:"phase"` // down, move, up, cancel
	Position *Position `json:"position,omitempty"`
}

// Trace is a data-only protocol and does NOT constitute server attestation.
// A future compiler/runtime rollout must declare its own v2 capability.
type Trace struct {
	Schema        string  `json:"schema"`
	DurationTicks uint32  `json:"durationTicks"`
	Events        []Event `json:"events"`
}

func bindingKind(ir *gameir.Document, action, device string) (string, error) {
	if ir == nil {
		return "", errors.New("missing published Game IR")
	}
	kind := ""
	for _, binding := range ir.InputBindings {
		if binding.Action != action {
			continue
		}
		isPointer := binding.Input == "tap" || binding.Input == "drag"
		if (device == "pointer" && !isPointer) ||
			(device == "keyboard" && binding.Input != "keyboard") {
			continue
		}
		// A single device/action cannot ambiguously mean both tap and drag.
		if kind != "" && kind != binding.Input {
			return "", fmt.Errorf("ambiguous published action binding")
		}
		kind = binding.Input
	}
	if kind == "" {
		return "", fmt.Errorf("semantic action is not declared by Game IR")
	}
	return kind, nil
}

// Validate is deliberately strict: no inferred releases, discarded events,
// implicit axes, or duplicate down edges. Older v1 logs are *not* parsed
// through this new grammar and retain their previous diagnostic behavior.
func Validate(ir *gameir.Document, trace Trace) error {
	if ir == nil {
		return errors.New("missing published Game IR")
	}
	if trace.Schema != SchemaV2 {
		return fmt.Errorf("unsupported semantic trace schema")
	}
	if trace.DurationTicks == 0 || trace.DurationTicks > MaxDurationTicks {
		return fmt.Errorf("semantic trace duration exceeds fixed-step bounds")
	}
	if len(trace.Events) > MaxEvents {
		return fmt.Errorf("semantic trace has too many events")
	}
	pressed := make(map[string]bool)
	var previousTick uint32
	var nextOrdinal uint32
	for index, event := range trace.Events {
		if event.Tick > trace.DurationTicks {
			return fmt.Errorf("semantic input tick exceeds trace duration")
		}
		if index == 0 || event.Tick != previousTick {
			if index > 0 && event.Tick < previousTick {
				return fmt.Errorf("semantic input ticks are not ordered")
			}
			nextOrdinal = 0
		}
		if event.Ordinal != nextOrdinal {
			return fmt.Errorf("semantic input ordinals must be consecutive within tick")
		}
		previousTick = event.Tick
		nextOrdinal++
		if event.Action == "" || len(event.Action) > maxActionBytes {
			return fmt.Errorf("semantic input action identifier is invalid")
		}
		if event.Device != "pointer" && event.Device != "keyboard" {
			return fmt.Errorf("unsupported semantic input device")
		}
		kind, err := bindingKind(ir, event.Action, event.Device)
		if err != nil {
			return err
		}
		key := event.Device + "\x00" + event.Action
		switch event.Phase {
		case "down":
			if pressed[key] {
				return fmt.Errorf("duplicate semantic input down")
			}
			pressed[key] = true
		case "move":
			if !pressed[key] || kind != "drag" {
				return fmt.Errorf("semantic move requires a held drag")
			}
		case "up", "cancel":
			if !pressed[key] {
				return fmt.Errorf("semantic release has no matching down")
			}
			delete(pressed, key)
		default:
			return fmt.Errorf("unknown semantic input phase")
		}
		if event.Device == "pointer" &&
			(event.Phase == "down" || event.Phase == "move") {
			if event.Position == nil {
				return fmt.Errorf("pointer input requires fixed-point position")
			}
			x, y := int64(event.Position.X), int64(event.Position.Y)
			if x < -MaxFixedCoordinate || x > MaxFixedCoordinate ||
				y < -MaxFixedCoordinate || y > MaxFixedCoordinate {
				return fmt.Errorf("pointer input coordinate out of bounds")
			}
		} else if event.Position != nil {
			// A pointer release has no coordinates; a keyboard input never
			// carries a position. Cancel is a state edge, not a motion.
			return fmt.Errorf("semantic input has an unexpected position")
		}
	}
	if len(pressed) != 0 {
		return fmt.Errorf("semantic trace leaves controls pressed")
	}
	return nil
}

// CanonicalSHA256 keys a validated trace to a published full IR digest and
// immutable seed. This SHA detects changed evidence; it is not a signature,
// a proof the browser performed the actions, or an authoritative score.
func CanonicalSHA256(ir *gameir.Document, gameID, fullIRSHA256 string, seed uint32, trace Trace) (string, error) {
	if gameID == "" || len(fullIRSHA256) != 64 {
		return "", errors.New("missing immutable semantic replay identity")
	}
	for _, ch := range fullIRSHA256 {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return "", errors.New("invalid full Game IR SHA-256")
		}
	}
	if err := Validate(ir, trace); err != nil {
		return "", err
	}
	// Persist a single canonical wire representation for zero-input
	// sessions; nil and an empty event list must fingerprint identically.
	if trace.Events == nil {
		trace.Events = []Event{}
	}
	type evidence struct {
		Schema   string `json:"schema"`
		GameID   string `json:"gameId"`
		IRSHA256 string `json:"irSha256"`
		Seed     uint32 `json:"seed"`
		Trace    Trace  `json:"trace"`
	}
	// JSON struct field order is stable and the protocol contains no
	// floating point, map iteration, or platform timestamps.
	raw, err := json.Marshal(evidence{SchemaV2, gameID, fullIRSHA256, seed, trace})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
