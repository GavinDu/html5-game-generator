package attestation

import (
	"encoding/json"
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
)

func TestStartScoreDiagnosticPreservesAuthoredRuleAndActionOrder(t *testing.T) {
	doc := minimalStartScoreIR()
	doc.Rules[0].Actions = []gameir.Action{
		{Type: "add_score", Value: float64(3)},
		{Type: "add_score", Value: float64(-2)},
		{Type: "add_score", Value: float64(7)},
	}
	doc.Rules = append(doc.Rules, gameir.Rule{
		ID: "second", When: gameir.Condition{Event: "game_start"},
		Actions: []gameir.Action{
			{Type: "add_score", Value: float64(-15)},
			{Type: "add_score", Value: float64(1)},
		},
	})
	if err := gameir.Validate(doc); err != nil {
		t.Fatalf("fixture must be valid published Game IR: %v", err)
	}
	got := reduceStartScoreDiagnostic(doc)
	if got.Admission.Kind != replayIRAdmissible ||
		got.Admission.Capability != replayStartScoreProfileV1 ||
		got.Admission.MaxActions != 5 || got.Score != -6 {
		t.Fatalf("unexpected reducer admission/score: %+v", got)
	}
	want := []startScoreStep{
		{Ordinal: 0, RuleID: "start_score", ActionIndex: 0, Delta: 3, Score: 3},
		{Ordinal: 1, RuleID: "start_score", ActionIndex: 1, Delta: -2, Score: 1},
		{Ordinal: 2, RuleID: "start_score", ActionIndex: 2, Delta: 7, Score: 8},
		{Ordinal: 3, RuleID: "second", ActionIndex: 0, Delta: -15, Score: -7},
		{Ordinal: 4, RuleID: "second", ActionIndex: 1, Delta: 1, Score: -6},
	}
	if !reflect.DeepEqual(got.Steps, want) {
		t.Fatalf("ordered game_start action ledger: got %+v; want %+v", got.Steps, want)
	}
}

func TestStartScoreDiagnosticStableAcrossFreshStrictIRDecode(t *testing.T) {
	doc := minimalStartScoreIR()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		parsed, err := gameir.DecodeAndValidate(encoded)
		if err != nil {
			t.Fatalf("independently decoded IR iteration %d: %v", i, err)
		}
		got := reduceStartScoreDiagnostic(parsed)
		if got.Admission.Kind != replayIRAdmissible || got.Score != 3 ||
			!reflect.DeepEqual(got, reduceStartScoreDiagnostic(doc)) {
			t.Fatalf("determinism regression at iteration %d: %+v", i, got)
		}
	}
}

func TestStartScoreDiagnosticDeniesUnsupportedWithoutPartialSteps(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gameir.Document)
	}{
		{"unsupported_collision", func(d *gameir.Document) {
			d.Rules[0].When.Event = "collision"
		}},
		{"unsupported_timer", func(d *gameir.Document) {
			d.Rules[0].When.Event = "timer_elapsed"
		}},
		{"unsupported_predicate", func(d *gameir.Document) {
			d.Rules[0].Conditions = []gameir.Predicate{
				{Variable: "score", Operator: "eq", Value: 0},
			}
		}},
		{"unsupported_float", func(d *gameir.Document) {
			d.Rules[0].Actions[0].Value = 1.5
		}},
		{"unsupported_numeric_string", func(d *gameir.Document) {
			d.Rules[0].Actions[0].Value = "3"
		}},
		{"unsupported_bool", func(d *gameir.Document) {
			d.Rules[0].Actions[0].Value = true
		}},
		{"unsupported_scene_change", func(d *gameir.Document) {
			d.Scenes = append(d.Scenes, gameir.Scene{ID: "other"})
		}},
		{"unsupported_input", func(d *gameir.Document) {
			d.InputBindings = []gameir.InputBinding{
				{Action: "tap", Input: "tap"},
			}
		}},
		{"unsupported_entity", func(d *gameir.Document) {
			d.Scenes[0].Entities = []gameir.Entity{
				{ID: "actor", Kind: "player", Position: []float64{0, 0}},
			}
		}},
		{"unsupported_game_end", func(d *gameir.Document) {
			d.Rules[0].Actions[0] = gameir.Action{Type: "end_game", Value: true}
		}},
		{"invalid_action", func(d *gameir.Document) {
			d.Rules[0].Actions[0].Type = "not_an_action"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := minimalStartScoreIR()
			tc.mutate(doc)
			got := reduceStartScoreDiagnostic(doc)
			if got.Admission.Kind == replayIRAdmissible || got.Score != 0 ||
				len(got.Steps) != 0 || got.Admission.Capability != "" {
				t.Fatalf("unsafe partial reducer for %s: %+v", tc.name, got)
			}
		})
	}
}

func TestStartScoreDiagnosticRejectsNilAndUnboundedRuleSet(t *testing.T) {
	got := reduceStartScoreDiagnostic(nil)
	if got.Admission.Kind != replayIRInvalid || got.Score != 0 ||
		len(got.Steps) != 0 {
		t.Fatalf("nil Game IR must deny without partial score: %+v", got)
	}
	doc := minimalStartScoreIR()
	for i := 0; i < 8; i++ {
		doc.Rules = append(doc.Rules, gameir.Rule{
			ID:      string(rune('a' + i)),
			When:    gameir.Condition{Event: "game_start"},
			Actions: []gameir.Action{{Type: "add_score", Value: 1000000}},
		})
	}
	got = reduceStartScoreDiagnostic(doc)
	if got.Admission.Kind == replayIRAdmissible || got.Score != 0 ||
		len(got.Steps) != 0 {
		t.Fatalf("rule budget bypass: %+v", got)
	}
}
