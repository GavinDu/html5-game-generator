package inventory

import (
	"math"
	"os"
	"testing"

	"doodlequest/internal/gameir"
)

func TestEscapeVersionTwoPacesItsTwentyMinuteQuestionnaire(t *testing.T) {
	bytes, err := os.ReadFile("../../inventory/2uowg9/game-ir.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := gameir.DecodeAndValidate(bytes)
	if err != nil {
		t.Fatalf("shipped Escape IR is invalid: %v", err)
	}
	if doc.Game.ID != "escape-from-injection" ||
		doc.Game.SessionTargetSeconds != 1200 || doc.Experience == nil ||
		doc.Experience.Pacing.TargetSessionSec != 1200 {
		t.Fatalf("game/pacing targets disagree with 20-minute authored story: %+v", doc.Game)
	}
	const stageSeconds = 40.0
	const sessionSeconds = 1200.0
	if len(doc.Experience.Pacing.Ramp) != 16 {
		t.Fatalf("expected original 40-second speed stages, got %d", len(doc.Experience.Pacing.Ramp))
	}
	for i, stage := range doc.Experience.Pacing.Ramp {
		actualSeconds := stage.At * sessionSeconds
		if math.Abs(actualSeconds-float64(i)*stageSeconds) > 0.001 {
			t.Fatalf("ramp stage %d changed its original 40-second timing: %g", i, actualSeconds)
		}
		if stage.SpeedMul > 3.2 || stage.SpeedMul < 1 {
			t.Fatalf("ramp stage %d violates the 1-3.2x speed safety cap: %g", i, stage.SpeedMul)
		}
	}
	caughtTerminal := false
	ceilingFound := false
	for _, rule := range doc.Rules {
		switch rule.ID {
		case "gap-caught":
			for _, action := range rule.Actions {
				if action.Type == "end_game" {
					if action.Value != false {
						t.Fatal("Escape may never gain a score-based victory")
					}
					caughtTerminal = true
				}
			}
		case "inevitable-catch":
			ceilingFound = rule.When.Event == "timer_elapsed" && rule.When.Value != nil &&
				*rule.When.Value == 1200
		}
	}
	if !caughtTerminal || !ceilingFound {
		t.Fatalf("authored caught-only ending or the 1200s ceiling was removed: caught=%v ceiling=%v",
			caughtTerminal, ceilingFound)
	}
}
