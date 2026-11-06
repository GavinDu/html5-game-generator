package pipeline

import (
	"strings"
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

func TestBuildRemixIRSupportsRepeatedRemixesAndTracksImmediateParent(t *testing.T) {
	originalIR := &gameir.Document{
		IRVersion: gameir.Version,
		Game: gameir.Game{ID: "original", Title: "Original", EntryScene: "field", Orientation: "portrait"},
		Assets: []gameir.Asset{{ID: "hero", Kind: "image", URI: "sprites/hero.png", MIMEType: "image/png"}},
		Scenes: []gameir.Scene{{ID: "field", Entities: []gameir.Entity{
			{ID: "player", Kind: "player", AssetID: "hero", Position: []float64{195, 700}, Size: []float64{100, 120}, Components: []string{"movement", "collider"}},
		}}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "tap"}},
		Rules: []gameir.Rule{{
			ID: "collect-heart",
			When: gameir.Condition{Event: "collected", Source: "player", Target: "heart"},
			Actions: []gameir.Action{{Type: "add_score", Target: "player", Value: 1}},
		}},
		Metadata: gameir.Metadata{Tags: []string{"family", "collecting"}},
	}
	original := &model.Game{ID: "original", Manifest: &model.BundleManifest{Version: 7}, GameIR: originalIR}

	firstChild := &model.Game{ID: "remix-one"}
	firstIR, err := buildRemixIR(&model.DesignDoc{Title: "First remix"}, firstChild, original)
	if err != nil {
		t.Fatalf("first remix: %v", err)
	}
	firstChild.Manifest = &model.BundleManifest{Version: 3}
	firstChild.GameIR = firstIR

	secondChild := &model.Game{ID: "remix-two"}
	secondIR, err := buildRemixIR(&model.DesignDoc{Title: "Second remix"}, secondChild, firstChild)
	if err != nil {
		t.Fatalf("second remix: %v", err)
	}

	if firstIR.Metadata.ParentGameID != original.ID || firstIR.Metadata.ParentVersion != original.Manifest.Version {
		t.Fatalf("first remix lineage changed: %+v", firstIR.Metadata)
	}
	if secondIR.Metadata.ParentGameID != firstChild.ID || secondIR.Metadata.ParentVersion != firstChild.Manifest.Version {
		t.Fatalf("second remix must point to its immediate parent/version: %+v", secondIR.Metadata)
	}
	if secondIR.Game.ID != secondChild.ID || secondIR.Game.Title != "Second remix" {
		t.Fatalf("second remix identity was not applied: %+v", secondIR.Game)
	}
	if len(secondIR.Assets) != 1 || secondIR.Assets[0].URI != "sprites/hero.png" ||
		len(secondIR.Scenes) != 1 || len(secondIR.Scenes[0].Entities) != 1 ||
		len(secondIR.Rules) != 1 || secondIR.Rules[0].ID != "collect-heart" ||
		len(secondIR.InputBindings) != 1 || secondIR.InputBindings[0].Input != "tap" {
		t.Fatal("repeated remix did not preserve the canonical gameplay loop and assets")
	}
	remixTags := 0
	for _, tag := range secondIR.Metadata.Tags {
		if tag == "remix" { remixTags++ }
	}
	if remixTags != 1 {
		t.Fatalf("repeated remix should contain exactly one remix tag, got %d: %v", remixTags, secondIR.Metadata.Tags)
	}
	if originalIR.Game.ID != "original" || originalIR.Metadata.ParentGameID != "" ||
		firstIR.Game.ID != firstChild.ID || firstIR.Metadata.ParentGameID != original.ID {
		t.Fatal("building a second-generation remix mutated an ancestor Game IR")
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

func TestBuildRemixIRUsesCanonicalBoboParentWithoutReplacingGameplay(t *testing.T) {
	parentIR := &gameir.Document{
		IRVersion: gameir.Version,
		Game:      gameir.Game{ID: "sample-rainbow-bear", Title: "Bobo", EntryScene: "rainbow-meadow", Orientation: "portrait"},
		Assets:    []gameir.Asset{{ID: "bear", Kind: "image", URI: "sprites/bear.png", MIMEType: "image/png"}},
		Scenes: []gameir.Scene{{
			ID: "rainbow-meadow",
			Entities: []gameir.Entity{
				{ID: "player", Kind: "player", AssetID: "bear", Position: []float64{195, 650}, Size: []float64{100, 120}, Components: []string{"movement", "collider", "score"}},
				{ID: "kitten", Kind: "decoration", Position: []float64{320, 650}, Size: []float64{70, 84}},
			},
		}},
		InputBindings: []gameir.InputBinding{{Action: "move", Input: "drag"}},
		Rules: []gameir.Rule{{
			ID:      "original-heart-loop",
			When:    gameir.Condition{Event: "collected", Source: "player", Target: "heart"},
			Actions: []gameir.Action{{Type: "add_score", Target: "player", Value: 1}},
		}},
		Metadata: gameir.Metadata{Tags: []string{"family", "collecting"}},
	}
	parent := &model.Game{
		ID:       "sample-rainbow-bear",
		Manifest: &model.BundleManifest{Version: 4},
		GameIR:   parentIR,
	}
	child := &model.Game{ID: "bobo-remix"}
	doc := &model.DesignDoc{Title: "Bobo's new adventure", Plot: "The same game with a new story"}

	remix, err := buildRemixIR(doc, child, parent)
	if err != nil {
		t.Fatalf("buildRemixIR() error = %v", err)
	}
	if remix.Game.EntryScene != "rainbow-meadow" || len(remix.Scenes) != 1 {
		t.Fatalf("Bobo remix replaced the parent's scene contract: %+v", remix)
	}
	if len(remix.Scenes[0].Entities) != 2 || remix.Scenes[0].Entities[0].ID != "player" {
		t.Fatalf("Bobo remix replaced the parent's entity loop: %+v", remix.Scenes[0].Entities)
	}
	if len(remix.Rules) != 1 || remix.Rules[0].ID != "original-heart-loop" {
		t.Fatalf("Bobo remix replaced parent gameplay rules: %+v", remix.Rules)
	}
	if remix.Metadata.ParentGameID != parent.ID || remix.Metadata.ParentVersion != parent.Manifest.Version {
		t.Fatalf("Bobo remix lineage missing: %+v", remix.Metadata)
	}
	if parentIR.Game.ID != "sample-rainbow-bear" || parentIR.Metadata.ParentGameID != "" {
		t.Fatal("Bobo remix mutated its canonical parent")
	}
}

func TestBoboRemixDesignDocUsesCanonicalParentExperience(t *testing.T) {
	parent := &model.Game{
		ID:       "sample-rainbow-bear",
		Title:    "Bobo's Rainbow",
		Manifest: &model.BundleManifest{Controls: []string{"tap"}},
		GameIR: &gameir.Document{
			Game: gameir.Game{Description: "Catch twelve hearts to restore six rainbow bands."},
			Experience: &gameir.Experience{
				Archetype: "catcher",
				Controls:  gameir.ExpControls{Scheme: "tap"},
				Objective: gameir.ExpObjective{Kind: "collect", Target: 12, Label: "Catch 12 hearts"},
				Failure:   gameir.ExpFailure{Mode: "gentle"},
			},
		},
	}
	doc := &model.DesignDoc{Title: "Generated placeholder", Plot: "Generated story"}
	applyRemixDesignDoc(doc, parent)

	if doc.Title != "Remix: Bobo's Rainbow" || doc.Plot != "Catch twelve hearts to restore six rainbow bands." {
		t.Fatalf("remix metadata not derived from parent: %+v", doc)
	}
	if doc.Mechanics == "" || len(doc.Controls) != 1 || doc.Controls[0] != "tap" || doc.WinCondition != "Catch 12 hearts" {
		t.Fatalf("remix questionnaire diverged from canonical experience: %+v", doc)
	}
	if !strings.Contains(doc.LoseCondition, "gentle") {
		t.Fatalf("gentle parent failure rules not preserved: %q", doc.LoseCondition)
	}
}

func TestBuildRemixIRRejectsLegacyBoboWithoutCanonicalGameIR(t *testing.T) {
	parent := &model.Game{
		ID:       "sample-rainbow-bear",
		Title:    "Bobo's Rainbow",
		Manifest: &model.BundleManifest{Version: 4},
	}
	child := &model.Game{ID: "bobo-remix"}
	_, err := buildRemixIR(&model.DesignDoc{Title: "Bobo Remix"}, child, parent)
	if err == nil {
		t.Fatal("legacy Bobo manifest without canonical Game IR must be rejected")
	}
	if !strings.Contains(err.Error(), "canonical Game IR") {
		t.Fatalf("error = %v, want canonical Game IR requirement", err)
	}
}
