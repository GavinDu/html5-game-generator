package pipeline

import (
	"reflect"
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
		{name: "bounded malformed target", doc: &model.DesignDoc{WinCondition: "Collect 999 stars", Plot: "Catch 5 stars"}, want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := targetScoreFromDesignDoc(tc.doc); got != tc.want {
				t.Fatalf("targetScoreFromDesignDoc() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildGameIRNormalizesInvalidObjectiveTargetAndLabel(t *testing.T) {
	doc := &model.DesignDoc{
		Title:        "Star Hunt",
		Plot:         "Catch 5 stars",
		WinCondition: "Collect 999 stars",
		Elements:     []model.Element{{ID: "el_1", Label: "dragon", Kind: "character"}},
	}
	ir := buildGameIR(doc, "invalid-target-game")
	if ir.Experience.Objective.Target != 10 {
		t.Fatalf("objective target = %d, want default 10", ir.Experience.Objective.Target)
	}
	if ir.Experience.Objective.Label != "Catch 10 stars" {
		t.Fatalf("objective label = %q, want normalized default", ir.Experience.Objective.Label)
	}
}

func TestBuildGenericRemixIRPreservesParentGameplayAndAppliesMetadataDelta(t *testing.T) {
	parentIR := &gameir.Document{
		IRVersion: gameir.Version12,
		Game: gameir.Game{
			ID: "parent-game", Title: "Original Game", Description: "Original story",
			EntryScene: "scene", Orientation: "portrait",
		},
		Assets: []gameir.Asset{{ID: "hero", Kind: "image", URI: "sprites/hero.png"}},
		Scenes: []gameir.Scene{{
			ID: "scene",
			Entities: []gameir.Entity{{
				ID: "hero-entity", Kind: "player", AssetID: "hero",
				Position: []float64{20, 30}, Size: []float64{40, 50},
				Components: []string{"movement", "collider"},
			}},
		}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}},
		Rules:         []gameir.Rule{{ID: "preserve-win-loop"}},
		Metadata:      gameir.Metadata{Tags: []string{"arcade"}},
	}
	parent := &model.Game{
		ID: "parent-game", Title: "Original Game", GameIR: parentIR,
		Manifest: &model.BundleManifest{Version: 3},
	}
	child := &model.Game{ID: "child-game", ParentGameID: parent.ID}
	doc := &model.DesignDoc{Title: "My Remix", Plot: "A new story description"}

	remix, err := buildGenericRemixIR(doc, child, parent)
	if err != nil {
		t.Fatalf("buildGenericRemixIR() error = %v", err)
	}
	if remix.Game.ID != child.ID || remix.Game.Title != doc.Title || remix.Game.Description != doc.Plot {
		t.Fatalf("remix metadata not applied: %#v", remix.Game)
	}
	if !reflect.DeepEqual(remix.Assets, parentIR.Assets) ||
		!reflect.DeepEqual(remix.Scenes, parentIR.Scenes) ||
		!reflect.DeepEqual(remix.InputBindings, parentIR.InputBindings) ||
		!reflect.DeepEqual(remix.Rules, parentIR.Rules) {
		t.Fatal("generic remix changed parent gameplay data")
	}
	if remix.Metadata.ParentGameID != parent.ID || remix.Metadata.ParentVersion != 3 {
		t.Fatalf("parent lineage not recorded: %#v", remix.Metadata)
	}
	if !reflect.DeepEqual(remix.Metadata.Tags, []string{"arcade", "remix"}) {
		t.Fatalf("remix tags = %#v, want original tags plus remix", remix.Metadata.Tags)
	}
	if parentIR.Game.ID != "parent-game" || parentIR.Game.Title != "Original Game" {
		t.Fatal("building a remix mutated the parent IR")
	}
}
