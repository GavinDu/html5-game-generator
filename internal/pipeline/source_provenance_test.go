package pipeline

import (
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// Generated labels are not original drawing pixels. A title or questionnaire
// element must never turn a synthetic SpritePNG into a "kid-drawing" asset.
func TestSyntheticElementSpriteHasTruthfulSourceProvenance(t *testing.T) {
	st := store.NewMemoryStore()
	game := &model.Game{
		ID: "provenance-generated-sprite",
		Status: model.GameDraft,
		Elements: []model.Element{{ID: "el_1", Label: "penguin", Kind: "character"}},
		Intent: "Collect hearts",
	}
	st.CreateGame(game)
	runner := NewRunner(st)
	job := runner.CreateAndStart(game, game.Elements, game.Intent, nil)
	waitPipelineStatus(t, st, job.ID, model.JobAwaitingConfirm)
	if _, err := runner.Confirm(game.ID, nil); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	waitPipelineStatus(t, st, job.ID, model.JobDone)
	published, ok := st.GetGame(game.ID)
	if !ok || published.Manifest == nil {
		t.Fatal("published manifest missing")
	}
	found := false
	for _, sprite := range published.Manifest.Assets.Sprites {
		if sprite.ID != "el_1" {
			continue
		}
		found = true
		if sprite.Source != "generated" {
			t.Fatalf("synthetic sprite incorrectly attributed to source artwork: %q", sprite.Source)
		}
	}
	if !found {
		t.Fatal("expected the generated element sprite in published manifest")
	}
}
