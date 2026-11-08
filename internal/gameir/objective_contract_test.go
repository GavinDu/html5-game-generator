package gameir

import (
	"strings"
	"testing"
)

func validProgressObjectiveDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := DecodeAndValidate([]byte(v12Doc(t)))
	if err != nil {
		t.Fatalf("canonical 1.2 fixture invalid: %v", err)
	}
	doc.Experience.Objective.Kind = "collect"
	doc.Experience.Objective.Target = 12
	doc.Experience.Objective.Label = "Catch 12 hearts"
	doc.Experience.Objective.ProgressVariable = "hearts_collected"
	doc.Rules = append(doc.Rules,
		Rule{
			ID: "regular-heart",
			When: Condition{
				Event: "collected", Source: "player", Target: "gem",
			},
			Actions: []Action{{Type: "add_variable", Variable: "hearts_collected", Value: 1}},
		},
		Rule{
			ID: "only-twelve-win",
			When: Condition{Event: "variable_changed"},
			Conditions: []Predicate{{
				Variable: "hearts_collected", Operator: "eq", Value: float64(12),
			}},
			Actions: []Action{{Type: "end_game", Value: true}},
		},
	)
	if err := Validate(doc); err != nil {
		t.Fatalf("properly progress-gated collect experience should validate: %v", err)
	}
	return doc
}

func TestCollectObjectiveRequiresItsCounterRatherThanBonusScoreForWin(t *testing.T) {
	t.Run("valid counter gate", func(t *testing.T) {
		validProgressObjectiveDoc(t)
	})
	t.Run("score-triggered early win", func(t *testing.T) {
		doc := validProgressObjectiveDoc(t)
		doc.Rules[len(doc.Rules)-1].When.Event = "score_reached"
		if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "wins via") {
			t.Fatalf("score win should be rejected for a distinct collectible count, got %v", err)
		}
	})
	t.Run("wrong progress threshold", func(t *testing.T) {
		doc := validProgressObjectiveDoc(t)
		doc.Rules[len(doc.Rules)-1].Conditions[0].Value = float64(6)
		if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "wins before") {
			t.Fatalf("early win must be rejected, got %v", err)
		}
	})
	t.Run("no counter increment", func(t *testing.T) {
		doc := validProgressObjectiveDoc(t)
		doc.Rules = append(doc.Rules[:len(doc.Rules)-2], doc.Rules[len(doc.Rules)-1])
		if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "no positive increment") {
			t.Fatalf("unreachable objective should be rejected, got %v", err)
		}
	})
	t.Run("no winning terminal", func(t *testing.T) {
		doc := validProgressObjectiveDoc(t)
		doc.Rules = doc.Rules[:len(doc.Rules)-1]
		if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "no progress-gated winning rule") {
			t.Fatalf("unfinishable collect objective should be rejected, got %v", err)
		}
	})
}

func TestGentleExperienceNeverDeclaresAHiddenLossOrNonbooleanTerminal(t *testing.T) {
	doc := validProgressObjectiveDoc(t)
	doc.Experience.Failure.Mode = "gentle"
	doc.Rules = append(doc.Rules, Rule{
		ID: "bad-loss", When: Condition{Event: "game_start"},
		Actions: []Action{{Type: "end_game", Value: false}},
	})
	if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "no-fail experience") {
		t.Fatalf("kid-friendly gentle mode cannot declare a losing end: %v", err)
	}

	doc = validProgressObjectiveDoc(t)
	doc.Rules[len(doc.Rules)-1].Actions[0].Value = "false"
	if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "boolean value") {
		t.Fatalf("string false is truthy in JavaScript and must never become a winning end: %v", err)
	}
}

func TestNoFailObjectiveAlsoRejectsExplicitLossRules(t *testing.T) {
	doc := validProgressObjectiveDoc(t)
	doc.Experience.Failure.Mode = "none"
	doc.Rules = append(doc.Rules, Rule{
		ID: "secret-loss", When: Condition{Event: "timer_elapsed"},
		Actions: []Action{{Type: "end_game", Value: false}},
	})
	if err := Validate(doc); err == nil || !strings.Contains(err.Error(), "no-fail experience") {
		t.Fatalf("none mode must not contain an undisclosed loss: %v", err)
	}
}
