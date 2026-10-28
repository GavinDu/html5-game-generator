package pipeline

import (
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

func TestBuildGenericRemixIRPreservesParentGameplayLoopAndAssets(t *testing.T) {
	parentIR := &gameir.Document{
		IRVersion: gameir.Version,
		Game:      gameir.Game{ID: "parent-game", Title: "Parent", EntryScene: "arena", Orientation: "landscape"},
		Assets:    []gameir.Asset{{ID: "hero", Kind: "image", URI: "assets/hero.png", MIMEType: "image/png", SHA256: "abc123"}},
		Scenes: []gameir.Scene{{
			ID:       "arena",
			Entities: []gameir.Entity{{ID: "player", Kind: "player", AssetID: "hero", Position: []float64{10, 20}, Size: []float64{30, 40}, Components: []string{"movement", "collider"}}},
		}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}},
		Rules: []gameir.Rule{{
			ID:      "collect-star",
			When:    gameir.Condition{Event: "collected", Source: "player", Target: "star"},
			Actions: []gameir.Action{{Type: "add_score", Target: "player", Value: 1}, {Type: "destroy_entity", Target: "star"}},
		}},
		Metadata: gameir.Metadata{Tags: []string{"platformer", "collecting"}},
	}
	parent := &model.Game{ID: "parent-game", Manifest: &model.BundleManifest{Version: 7}, GameIR: parentIR}
	child := &model.Game{ID: "child-game"}
	doc := &model.DesignDoc{Title: "A remixed adventure", Plot: "Same loop, new story"}

	remix, err := buildGenericRemixIR(doc, child, parent)
	if err != nil {
		t.Fatalf("buildGenericRemixIR() error = %v", err)
	}
	if remix.Game.ID != child.ID || remix.Game.Title != doc.Title || remix.Game.Description != doc.Plot {
		t.Fatalf("remix identity/title/description not applied: %+v", remix.Game)
	}
	if remix.Metadata.ParentGameID != parent.ID || remix.Metadata.ParentVersion != parent.Manifest.Version {
		t.Fatalf("parent lineage not recorded: %+v", remix.Metadata)
	}
	if !containsRemixTag(remix.Metadata.Tags) {
		t.Fatalf("remix tag missing: %v", remix.Metadata.Tags)
	}
	if len(remix.Assets) != 1 || remix.Assets[0].ID != "hero" || remix.Assets[0].URI != "assets/hero.png" || remix.Assets[0].SHA256 != "abc123" {
		t.Fatalf("generic remix did not preserve parent asset identity and content reference: %+v", remix.Assets)
	}
	if len(remix.Scenes) != 1 || remix.Scenes[0].ID != "arena" || len(remix.Scenes[0].Entities) != 1 {
		t.Fatalf("generic remix did not preserve parent scene/entity structure: %+v", remix.Scenes)
	}
	entity := remix.Scenes[0].Entities[0]
	if entity.ID != "player" || entity.AssetID != "hero" || len(entity.Components) != 2 || entity.Components[0] != "movement" || entity.Components[1] != "collider" {
		t.Fatalf("generic remix changed parent gameplay entity: %+v", entity)
	}
	if len(remix.InputBindings) != 1 || remix.InputBindings[0].Action != "move" || remix.InputBindings[0].Input != "drag" {
		t.Fatalf("generic remix changed parent input bindings: %+v", remix.InputBindings)
	}
	if len(remix.Rules) != 1 || remix.Rules[0].ID != "collect-star" || len(remix.Rules[0].Actions) != 2 || remix.Rules[0].Actions[0].Type != "add_score" || remix.Rules[0].Actions[1].Type != "destroy_entity" {
		t.Fatalf("generic remix changed parent gameplay rules: %+v", remix.Rules)
	}
	if parentIR.Game.ID != "parent-game" || parentIR.Metadata.ParentGameID != "" {
		t.Fatal("building a remix mutated the parent Game IR")
	}
}

func containsRemixTag(tags []string) bool {
	for _, tag := range tags {
		if tag == "remix" {
			return true
		}
	}
	return false
}
