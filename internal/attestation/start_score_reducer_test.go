package attestation

import (
	"errors"
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
)

func TestReduceStartScoreV1ExecutesAuthoredRuleAndActionOrder(t *testing.T) {
	doc := minimalStartScoreIR()
	doc.Rules[0].Actions = []gameir.Action{
		{Type: "add_score", Value: float64(3)},
		{Type: "add_score", Value: float64(-8)},
		{Type: "add_score", Value: float64(1)},
	}
	doc.Rules = append(doc.Rules, gameir.Rule{
		ID:   "second",
		When: gameir.Condition{Event: "game_start"},
		Actions: []gameir.Action{
			{Type: "add_score", Value: float64(2)},
			{Type: "add_score", Value: float64(-2)},
		},
	})
	got, err := reduceStartScoreV1(doc)
	if err != nil {
		t.Fatalf("valid scalar start-only IR refused: %v", err)
	}
	want := startScoreResultV1{
		Capability: replayStartScoreProfileV1,
		Score:      -4,
		Steps: []startScoreStepV1{
			{RuleID: "start_score", ActionIndex: 0, ScoreAfter: 3},
			{RuleID: "start_score", ActionIndex: 1, ScoreAfter: -5},
			{RuleID: "start_score", ActionIndex: 2, ScoreAfter: -4},
			{RuleID: "second", ActionIndex: 0, ScoreAfter: -2},
			{RuleID: "second", ActionIndex: 1, ScoreAfter: -4},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("authored order differs: got %+v want %+v", got, want)
	}
	for attempt := 0; attempt < 32; attempt++ {
		repeat, repeatErr := reduceStartScoreV1(doc)
		if repeatErr != nil || !reflect.DeepEqual(repeat, want) {
			t.Fatalf("nonreproducible diagnostic reduction attempt %d: %+v %v", attempt, repeat, repeatErr)
		}
	}
}

func TestReduceStartScoreV1RejectsInvalidAndUnsupportedIRWithoutPartialScore(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*gameir.Document)
	}{
		{"unknown_action", func(d *gameir.Document) { d.Rules[0].Actions[0].Type = "not_a_rule" }},
		{"fractional_score", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = 0.125 }},
		{"numeric_string", func(d *gameir.Document) { d.Rules[0].Actions[0].Value = "3" }},
		{"timer", func(d *gameir.Document) { d.Rules[0].When.Event = "timer_elapsed" }},
		{"collision", func(d *gameir.Document) { d.Rules[0].When.Event = "collision" }},
		{"ending", func(d *gameir.Document) { d.Rules[0].Actions[0].Type = "end_game" }},
		{"game_input", func(d *gameir.Document) {
			d.InputBindings = []gameir.InputBinding{{Action: "tap", Input: "tap"}}
		}},
		{"nonempty_scene", func(d *gameir.Document) { d.Scenes[0].BackgroundColor = "#ffffff" }},
		{"no_rules", func(d *gameir.Document) { d.Rules = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := minimalStartScoreIR()
			tc.mutate(doc)
			got, err := reduceStartScoreV1(doc)
			if !errors.Is(err, errStartScoreNotAdmitted) {
				t.Fatalf("unsupported IR has no typed rejection: %+v err=%v", got, err)
			}
			if got.Capability != "" || len(got.Steps) != 0 || got.Score != 0 {
				t.Fatalf("partial score escaped rejected IR: %+v", got)
			}
		})
	}
	got, err := reduceStartScoreV1(nil)
	if !errors.Is(err, errStartScoreNotAdmitted) || len(got.Steps) != 0 {
		t.Fatalf("nil IR was not strictly rejected: %+v %v", got, err)
	}
}
