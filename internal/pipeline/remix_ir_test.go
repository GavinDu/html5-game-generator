package pipeline

import (
	"reflect"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
)

func TestBuildGenericRemixIRPreservesParentGameplayLoopAndAssets(t *testing.T) {
	parentIR := &gameir.Document{
		IRVersion: gameir.Version,
		Game: gameir.Game{ID: "parent-game", Title: "Parent", EntryScene: "arena", Orientation: "landscape"},
		Assets: []gameir.Asset{{ID: "hero", Kind: "image", URI: "assets/hero.png", MIMEType: "image/png", SHA256: "abc123"}},
		Scenes: []gameir.Scene{{
			ID: "arena",
			Entities: []gameir.Entity{{ID: "player", Kind: "player", AssetID: "hero", Position: []float64{10, 20}, Size: []float64{30, 40}, Components: []string{"movement", "collider"}}},
		}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}},
		Rules: []gameir.Rule{{
			ID: "collect-star",
			When: gameir.Condition{Event: "collected", Source: "player", Target: "star"},
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
	if !reflect.DeepEqual(remix.Assets, parentIR.Assets) ||
		!reflect.DeepEqual(remix.Scenes, parentIR.Scenes) ||
		!reflect.DeepEqual(remix.InputBindings, parentIR.InputBindings) ||
		!reflect.DeepEqual(remix.Rules, parentIR.Rules) {
		t.Fatal("generic remix changed the parent's assets, scenes, controls, or gameplay rules")
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
