package semanticreplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/replaytrace"
)

func tapScoreIR() *gameir.Document {
	return &gameir.Document{
		IRVersion: gameir.Version,
		Game: gameir.Game{
			ID: "tap-score", Title: "Tap Score",
			EntryScene: "main", Orientation: "any",
		},
		Scenes:        []gameir.Scene{{ID: "main"}},
		InputBindings: []gameir.InputBinding{{Action: "tap", Input: "tap"}},
		Rules: []gameir.Rule{{
			ID: "score", When: gameir.Condition{
				Event: "input", InputAction: "tap",
			},
			Actions: []gameir.Action{{Type: "add_score", Value: float64(2)}},
		}},
	}
}

func tapTrace(ticks ...uint32) replaytrace.Trace {
	events := make([]replaytrace.Event, 0, len(ticks)*2)
	var prev uint32
	var ordinal uint32
	for i, tick := range ticks {
		if i == 0 || tick != prev {
			ordinal = 0
		}
		events = append(events,
			replaytrace.Event{
				Tick: tick, Ordinal: ordinal, Action: "tap", Device: "pointer",
				Phase: "down", Position: &replaytrace.Position{X: 1000, Y: 2000},
			},
			replaytrace.Event{
				Tick: tick, Ordinal: ordinal + 1, Action: "tap", Device: "pointer",
				Phase: "up",
			},
		)
		ordinal += 2
		prev = tick
	}
	duration := uint32(20)
	if len(ticks) > 0 && ticks[len(ticks)-1] > duration {
		duration = ticks[len(ticks)-1]
	}
	return replaytrace.Trace{
		Schema: replaytrace.SchemaV2, DurationTicks: duration, Events: events,
	}
}

func TestTapScoreSupportedIRAndCanonicalDiagnostic(t *testing.T) {
	ir := tapScoreIR()
	if err := gameir.Validate(ir); err != nil {
		t.Fatalf("fixture should be valid declarative Game IR: %v", err)
	}
	if err := SupportedIR(ir); err != nil {
		t.Fatalf("tap-only IR unexpectedly rejected: %v", err)
	}
	trace := tapTrace(1, 1, 2)
	a, err := ReplayDiagnostic(ir, trace, 17)
	if err != nil {
		t.Fatal(err)
	}
	if a.Score != 6 || a.Terminal != "running" ||
		a.DownEvents != 3 || a.ActionsApplied != 3 ||
		a.Attested || a.EvidenceStatus != "unverified" || a.RankingScope != "none" ||
		a.ProofVersion != ProofVersion ||
		len(a.StateSHA256) != 64 || len(a.TraceSHA256) != 64 {
		t.Fatalf("diagnostic contract drift: %+v", a)
	}
	b, err := ReplayDiagnostic(ir, trace, 17)
	if err != nil || a != b {
		t.Fatalf("same seed/input should repeat exactly: A=%+v B=%+v err=%v", a, b, err)
	}
	c, err := ReplayDiagnostic(ir, trace, 18)
	if err != nil || c.Score != a.Score || c.StateSHA256 == a.StateSHA256 ||
		c.TraceSHA256 == a.TraceSHA256 {
		t.Fatalf("seed must pin canonical evidence without mutating score: %+v err=%v", c, err)
	}
	zero, err := ReplayDiagnostic(ir, replaytrace.Trace{
		Schema: replaytrace.SchemaV2, DurationTicks: 20,
	}, 17)
	if err != nil || zero.Score != 0 || zero.Terminal != "running" ||
		zero.DownEvents != 0 || zero.ActionsApplied != 0 {
		t.Fatalf("zero-input diagnostic must remain uncompleted: %+v err=%v", zero, err)
	}
	// nil and [] represent identical absence of semantic inputs.
	empty, err := ReplayDiagnostic(ir, replaytrace.Trace{
		Schema: replaytrace.SchemaV2, DurationTicks: 20,
		Events: []replaytrace.Event{},
	}, 17)
	if err != nil || empty != zero {
		t.Fatalf("nil/empty trace canonicalization mismatch: %+v %+v %v", empty, zero, err)
	}
}

func TestTapScoreExplicitTerminalStopsFurtherSameTickAndLaterActions(t *testing.T) {
	ir := tapScoreIR()
	ir.Rules = []gameir.Rule{
		{ID: "first", When: gameir.Condition{Event: "input", InputAction: "tap"},
			Actions: []gameir.Action{{Type: "add_score", Value: float64(-2)}}},
		{ID: "stop", When: gameir.Condition{Event: "input", InputAction: "tap"},
			Actions: []gameir.Action{{Type: "end_game", Value: true}}},
		{ID: "never", When: gameir.Condition{Event: "input", InputAction: "tap"},
			Actions: []gameir.Action{{Type: "add_score", Value: float64(100)}}},
	}
	result, err := ReplayDiagnostic(ir, tapTrace(1, 1, 3), 42)
	if err != nil || result.Score != -2 || result.Terminal != "win" ||
		result.DownEvents != 1 || result.ActionsApplied != 2 {
		t.Fatalf("terminal result must be immutable: %+v err=%v", result, err)
	}
	ir.Rules[1].Actions[0].Value = false
	result, err = ReplayDiagnostic(ir, tapTrace(1), 42)
	if err != nil || result.Score != -2 || result.Terminal != "lose" {
		t.Fatalf("explicit false terminal expected: %+v err=%v", result, err)
	}
}

func TestTapScoreRejectsUnsupportedIRWithoutPartialExecution(t *testing.T) {
	cases := []struct {
		name   string
		change func(*gameir.Document)
		want   error
	}{
		{"nil-experience-disallowed-v12", func(d *gameir.Document) { d.IRVersion = gameir.Version12 }, ErrInvalidIR},
		{"version-11", func(d *gameir.Document) { d.IRVersion = gameir.Version11 }, ErrUnsupportedIR},
		{"session-timer", func(d *gameir.Document) { d.Game.SessionTargetSeconds = 60 }, ErrUnsupportedIR},
		{"gravity", func(d *gameir.Document) { d.Scenes[0].Gravity = []float64{0, 9.8} }, ErrUnsupportedIR},
		{"world-size", func(d *gameir.Document) { d.Scenes[0].WorldSize = []float64{400, 400} }, ErrUnsupportedIR},
		{"second-scene", func(d *gameir.Document) { d.Scenes = append(d.Scenes, gameir.Scene{ID: "second"}) }, ErrUnsupportedIR},
		{"entity", func(d *gameir.Document) {
			d.Scenes[0].Entities = []gameir.Entity{{ID: "player", Kind: "player", Position: []float64{0, 0}}}
		}, ErrUnsupportedIR},
		{"keyboard", func(d *gameir.Document) { d.InputBindings[0].Input = "keyboard"; d.InputBindings[0].Key = "Space" }, ErrUnsupportedInput},
		{"drag", func(d *gameir.Document) { d.InputBindings[0].Input = "drag" }, ErrUnsupportedInput},
		{"condition", func(d *gameir.Document) {
			d.Rules[0].Conditions = []gameir.Predicate{{Variable: "score", Operator: "gte", Value: float64(0)}}
		}, ErrUnsupportedRule},
		{"wrong-rule-event", func(d *gameir.Document) { d.Rules[0].When.Event = "score_reached" }, ErrUnsupportedRule},
		{"missing-input-action", func(d *gameir.Document) { d.Rules[0].When.InputAction = "" }, ErrUnsupportedRule},
		{"health", func(d *gameir.Document) {
			d.Rules[0].Actions[0] = gameir.Action{Type: "change_health", Value: float64(-1)}
		}, ErrUnsupportedAction},
		{"string-score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = "10" }, ErrUnsupportedAction},
		{"fractional-score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = float64(1.5) }, ErrUnsupportedAction},
		{"nan-score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = float64(1e100) }, ErrUnsupportedAction},
		{"implicit-end-game", func(d *gameir.Document) { d.Rules[0].Actions[0] = gameir.Action{Type: "end_game"} }, ErrUnsupportedAction},
		{"action-effect-extra", func(d *gameir.Document) { d.Rules[0].Actions[0].DurationMS = 100 }, ErrUnsupportedAction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ir := tapScoreIR()
			tc.change(ir)
			if err := SupportedIR(ir); !errors.Is(err, tc.want) {
				t.Fatalf("unsupported IR err=%v expected %v", err, tc.want)
			}
			result, err := ReplayDiagnostic(ir, tapTrace(1), 3)
			if !errors.Is(err, tc.want) || result != (Diagnostic{}) {
				t.Fatalf("must refuse all effects on unsupported profile, result=%+v err=%v", result, err)
			}
		})
	}
	if err := SupportedIR(nil); !errors.Is(err, ErrInvalidIR) {
		t.Fatalf("missing IR must reject: %v", err)
	}
}

func TestTapScoreRejectsInvalidCanonicalEdgesAndBounds(t *testing.T) {
	ir := tapScoreIR()
	cases := []struct {
		name  string
		trace replaytrace.Trace
	}{
		{"duplicate-down", replaytrace.Trace{
			Schema: replaytrace.SchemaV2, DurationTicks: 10,
			Events: []replaytrace.Event{
				{Tick: 1, Ordinal: 0, Action: "tap", Device: "pointer", Phase: "down", Position: &replaytrace.Position{}},
				{Tick: 1, Ordinal: 1, Action: "tap", Device: "pointer", Phase: "down", Position: &replaytrace.Position{}},
			},
		}},
		{"out-of-order", replaytrace.Trace{
			Schema: replaytrace.SchemaV2, DurationTicks: 10,
			Events: []replaytrace.Event{
				{Tick: 2, Ordinal: 0, Action: "tap", Device: "pointer", Phase: "down", Position: &replaytrace.Position{}},
				{Tick: 1, Ordinal: 0, Action: "tap", Device: "pointer", Phase: "up"},
			},
		}},
		{"unreleased", replaytrace.Trace{
			Schema: replaytrace.SchemaV2, DurationTicks: 10,
			Events: []replaytrace.Event{{Tick: 1, Ordinal: 0, Action: "tap", Device: "pointer", Phase: "down", Position: &replaytrace.Position{}}},
		}},
		{"wrong-action", replaytrace.Trace{
			Schema: replaytrace.SchemaV2, DurationTicks: 10,
			Events: []replaytrace.Event{{Tick: 1, Ordinal: 0, Action: "forged", Device: "pointer", Phase: "down", Position: &replaytrace.Position{}}},
		}},
		{"zero-duration", replaytrace.Trace{Schema: replaytrace.SchemaV2}},
		{"too-long", replaytrace.Trace{Schema: replaytrace.SchemaV2, DurationTicks: replaytrace.MaxDurationTicks + 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ReplayDiagnostic(ir, tc.trace, 5)
			if !errors.Is(err, ErrInvalidTrace) || result != (Diagnostic{}) {
				t.Fatalf("malformed trace accepted: %+v %v", result, err)
			}
		})
	}
	// Work limits prevent a large but syntactically valid tap trace and rule
	// set from turning semantic replay into an unbounded CPU workload.
	ir.Rules = nil
	for i := 0; i < maxRules; i++ {
		actions := make([]gameir.Action, 16)
		for j := range actions {
			actions[j] = gameir.Action{Type: "add_score", Value: float64(1)}
		}
		ir.Rules = append(ir.Rules, gameir.Rule{
			ID:      fmt.Sprintf("score%d", i),
			When:    gameir.Condition{Event: "input", InputAction: "tap"},
			Actions: actions,
		})
	}
	ticks := make([]uint32, 120)
	for i := range ticks {
		ticks[i] = uint32(i + 1)
	}
	result, err := ReplayDiagnostic(ir, tapTrace(ticks...), 1)
	if !errors.Is(err, ErrWorkBudget) || result != (Diagnostic{}) {
		t.Fatalf("semantic work must be bounded: %+v %v", result, err)
	}
}

func TestTapScoreDiagnosticNeverClaimsServerAttestation(t *testing.T) {
	ir := tapScoreIR()
	result, err := ReplayDiagnostic(ir, tapTrace(1), 123)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"attested":true`) || !strings.Contains(string(raw), `"rankingScope":"none"`) ||
		!strings.Contains(string(raw), `"evidenceStatus":"unverified"`) {
		t.Fatalf("diagnostic replay must not be promoted to a verdict: %s", raw)
	}
}
