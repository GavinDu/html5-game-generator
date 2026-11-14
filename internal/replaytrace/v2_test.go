package replaytrace

import (
	"encoding/hex"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
)

func sampleIR() *gameir.Document {
	return &gameir.Document{InputBindings: []gameir.InputBinding{
		{Input: "tap", Action: "jump"},
		{Input: "drag", Action: "aim"},
		{Input: "keyboard", Action: "move_left", Key: "ArrowLeft"},
	}}
}

func pos(x, y int32) *Position { return &Position{X: x, Y: y} }

func validTrace() Trace {
	return Trace{
		Schema: SchemaV2, DurationTicks: 120,
		Events: []Event{
			{Tick: 1, Ordinal: 0, Action: "jump", Device: "pointer", Phase: "down", Position: pos(1000, 2000)},
			{Tick: 1, Ordinal: 1, Action: "jump", Device: "pointer", Phase: "up"},
			{Tick: 2, Ordinal: 0, Action: "aim", Device: "pointer", Phase: "down", Position: pos(5000, 6000)},
			{Tick: 3, Ordinal: 0, Action: "aim", Device: "pointer", Phase: "move", Position: pos(6000, 7000)},
			{Tick: 4, Ordinal: 0, Action: "aim", Device: "pointer", Phase: "cancel"},
			{Tick: 4, Ordinal: 1, Action: "move_left", Device: "keyboard", Phase: "down"},
			{Tick: 5, Ordinal: 0, Action: "move_left", Device: "keyboard", Phase: "up"},
		},
	}
}

func TestSemanticTraceV2ValidatesDeclaredEdgesAndCanonicalIdentity(t *testing.T) {
	ir := sampleIR()
	trace := validTrace()
	if err := Validate(ir, trace); err != nil {
		t.Fatalf("valid semantic trace: %v", err)
	}
	const full = "abababababababababababababababababababababababababababababababab"
	reference, err := CanonicalSHA256(ir, "game-1", full, 42, trace)
	if err != nil || len(reference) != 64 {
		t.Fatalf("canonical v2 digest %q: %v", reference, err)
	}
	if _, err := hex.DecodeString(reference); err != nil {
		t.Fatal(err)
	}
	same, err := CanonicalSHA256(ir, "game-1", full, 42, trace)
	if err != nil || same != reference {
		t.Fatalf("repeated input must have deterministic hash: %q != %q", same, reference)
	}
	for _, test := range []struct {
		name string
		game string
		full string
		seed uint32
		trace Trace
	}{
		{"different-game", "game-2", full, 42, trace},
		{"different-ir", "game-1", strings.Repeat("c", 64), 42, trace},
		{"different-seed", "game-1", full, 43, trace},
		{"different-duration", "game-1", full, 42, func() Trace { c := trace; c.DurationTicks++; return c }()},
		{"removed-input", "game-1", full, 42, func() Trace { c := trace; c.Events = append([]Event(nil), trace.Events[:len(trace.Events)-2]...); return c }()},
		{"different-position", "game-1", full, 42, func() Trace {
			c := trace
			c.Events = append([]Event(nil), trace.Events...)
			c.Events[0].Position = pos(3000, 2000)
			return c
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			fingerprint, err := CanonicalSHA256(ir, test.game, test.full, test.seed, test.trace)
			// Deleting a terminal key release is invalid and must fail, not
			// silently receive a different trusted fingerprint.
			if test.name == "removed-input" {
				if err == nil {
					t.Fatal("partial trace unexpectedly admitted")
				}
			} else if err != nil || fingerprint == reference {
				t.Fatalf("mutated evidence not distinguished: %q == %q: %v", fingerprint, reference, err)
			}
		})
	}
	_, err = CanonicalSHA256(ir, "game-1", "short", 42, trace)
	if err == nil {
		t.Fatal("a 12-hex board key is not a full immutable IR digest")
	}
	_, err = CanonicalSHA256(ir, "game-1", strings.Repeat("G", 64), 42, trace)
	if err == nil {
		t.Fatal("uppercase or nonhex full digest must fail")
	}
}

func TestSemanticTraceV2RejectsMalformedAndUnbalancedActions(t *testing.T) {
	valid := validTrace()
	tests := []struct {
		name string
		mutate func(*Trace)
	}{
		{"unknown-schema", func(x *Trace) { x.Schema = "semantic-input-trace-v9" }},
		{"zero-duration", func(x *Trace) { x.DurationTicks = 0 }},
		{"excess-duration", func(x *Trace) { x.DurationTicks = MaxDurationTicks + 1 }},
		{"tick-after-duration", func(x *Trace) { x.DurationTicks = 2 }},
		{"backwards-tick", func(x *Trace) { x.Events[3].Tick = 1 }},
		{"gap-ordinal", func(x *Trace) { x.Events[1].Ordinal = 3 }},
		{"duplicate-ordinal", func(x *Trace) { x.Events[1].Ordinal = 0 }},
		{"undeclared-action", func(x *Trace) { x.Events[0].Action = "teleport" }},
		{"unsupported-device", func(x *Trace) { x.Events[0].Device = "gamepad" }},
		{"tap-move", func(x *Trace) { x.Events[1].Phase = "move"; x.Events[1].Position = pos(1000, 1000) }},
		{"keyboard-pointer", func(x *Trace) { x.Events[5].Device = "pointer"; x.Events[5].Position = pos(0, 0) }},
		{"duplicate-down", func(x *Trace) { x.Events[1].Phase = "down"; x.Events[1].Position = pos(10, 20) }},
		{"orphan-release", func(x *Trace) { x.Events[0].Phase = "up"; x.Events[0].Position = nil }},
		{"missing-release", func(x *Trace) { x.Events = x.Events[:len(x.Events)-1] }},
		{"unknown-phase", func(x *Trace) { x.Events[0].Phase = "swipe" }},
		{"missing-pointer-position", func(x *Trace) { x.Events[0].Position = nil }},
		{"unexpected-key-position", func(x *Trace) { x.Events[5].Position = pos(1, 1) }},
		{"unexpected-release-position", func(x *Trace) { x.Events[1].Position = pos(1, 1) }},
		{"coordinate-overflow", func(x *Trace) { x.Events[0].Position = pos(MaxFixedCoordinate + 1, 0) }},
		{"oversized-action", func(x *Trace) { x.Events[0].Action = strings.Repeat("a", 129) }},
		{"too-many-events", func(x *Trace) { x.Events = make([]Event, MaxEvents+1) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x := valid
			x.Events = append([]Event(nil), valid.Events...)
			tc.mutate(&x)
			if err := Validate(sampleIR(), x); err == nil {
				t.Fatalf("invalid v2 semantic trace accepted: %+v", x)
			}
		})
	}
	if err := Validate(nil, valid); err == nil {
		t.Fatal("missing published IR must reject all events")
	}
}

func TestSemanticTraceV2EmptyAndLegacyCompatibilityBoundary(t *testing.T) {
	ir := sampleIR()
	// Time-only valid game can have no inputs, but *unbalanced* event
	// sequences cannot be declared complete by an untrusted browser.
	const full = "abababababababababababababababababababababababababababababababab"
	withNil := Trace{Schema: SchemaV2, DurationTicks: 10}
	withEmpty := Trace{Schema: SchemaV2, DurationTicks: 10, Events: []Event{}}
	nilHash, err := CanonicalSHA256(ir, "game", full, 0, withNil)
	if err != nil {
		t.Fatal(err)
	}
	emptyHash, err := CanonicalSHA256(ir, "game", full, 0, withEmpty)
	if err != nil || nilHash != emptyHash {
		t.Fatalf("empty v2 trace must canonicalize nil/[]: %q != %q, %v", nilHash, emptyHash, err)
	}
	// This package neither rewrites nor makes new verified claims about
	// stored RunInput v1 traces; future protocol negotiation is required.
	if err := Validate(ir, Trace{Schema: "replay-input-evidence-v1", DurationTicks: 10}); err == nil {
		t.Fatal("new v2 validator must not silently reinterpret v1 data")
	}
}
