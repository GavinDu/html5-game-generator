package pipeline

import (
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

func TestBuildGameIREmitsPlayableVersion12Catcher(t *testing.T) {
	doc := &model.DesignDoc{
		Title:         "Dragon's Star Hunt",
		Plot:          "Catch falling stars with a dragon.",
		Mechanics:     "Drag left and right to catch falling stars.",
		WinCondition:  "Catch 7 stars",
		LoseCondition: "None — endless until you win",
		Elements:      []model.Element{{ID: "el_1", Label: "dragon", Kind: "character"}},
		MusicMood:     "adventurous",
	}
	ir := buildGameIR(doc, "test-game")
	if err := gameir.Validate(ir); err != nil {
		t.Fatalf("generated IR must validate: %v", err)
	}
	if ir.IRVersion != gameir.Version12 {
		t.Fatalf("IR version = %q, want %q", ir.IRVersion, gameir.Version12)
	}
	if ir.Experience == nil || ir.Experience.Archetype != "catcher" {
		t.Fatalf("expected catcher experience block, got %#v", ir.Experience)
	}
	if ir.Experience.Objective.Target != 7 {
		t.Fatalf("objective target = %d, want 7", ir.Experience.Objective.Target)
	}
	if !strings.Contains(ir.Experience.Objective.Label, "7") {
		t.Fatalf("objective label should come from confirmed design doc: %q", ir.Experience.Objective.Label)
	}

	var hasSpawnTemplate, hasTimerSpawn, hasThemeStart, hasWinRule bool
	for _, scene := range ir.Scenes {
		for _, template := range scene.Templates {
			if template.ID == "star-pickup-template" && template.Behavior != nil && template.Behavior.Type == "fall" {
				hasSpawnTemplate = true
			}
		}
	}
	for _, rule := range ir.Rules {
		switch rule.ID {
		case "spawn-falling-stars":
			hasTimerSpawn = rule.When.Event == "timer_elapsed" && len(rule.Actions) > 0 && rule.Actions[0].Type == "spawn_entity" && rule.Actions[0].Target == "star-pickup-template"
		case "start-theme":
			hasThemeStart = rule.When.Event == "game_start" && len(rule.Actions) > 0 && rule.Actions[0].Type == "play_sound" && rule.Actions[0].AssetID == "theme"
		case "finish-game":
			hasWinRule = rule.When.Event == "score_reached" && rule.When.Value != nil && int(*rule.When.Value) == ir.Experience.Objective.Target
		}
	}
	if !hasSpawnTemplate || !hasTimerSpawn {
		t.Fatalf("generated game must replenish pickups: template=%v timer=%v", hasSpawnTemplate, hasTimerSpawn)
	}
	if !hasThemeStart {
		t.Fatal("generated game must start theme audio on game_start")
	}
	if !hasWinRule {
		t.Fatal("win rule threshold must match the confirmed objective target")
	}
}

func TestTargetScoreFromDesignDocDefaultsAndBounds(t *testing.T) {
	cases := []struct {
		name string
		doc  *model.DesignDoc
		want int
	}{
		{name: "default", doc: &model.DesignDoc{WinCondition: "Catch stars"}, want: 10},
		{name: "confirmed objective", doc: &model.DesignDoc{WinCondition: "Collect 12 stars"}, want: 12},
		{name: "bounded malformed target", doc: &model.DesignDoc{WinCondition: "Collect 999 stars", Plot: "Catch 5 stars"}, want: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := targetScoreFromDesignDoc(tc.doc); got != tc.want {
				t.Fatalf("targetScoreFromDesignDoc() = %d, want %d", got, tc.want)
			}
		})
	}
}
