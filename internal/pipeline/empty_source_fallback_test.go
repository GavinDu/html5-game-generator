package pipeline

import (
	"bytes"
	"testing"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// A zero-element confirmation used to publish a valid-looking canonical IR
// referencing sprites/el_1.png without writing that PNG or listing it in the
// manifest. The game loaded with a missing hero on device.
func TestEmptyConfirmedQuestionnairePublishesResolvableFallbackPlayer(t *testing.T) {
	st := store.NewMemoryStore()
	game := &model.Game{
		ID:     "empty-source-creator",
		Title:  "Before confirmation",
		Status: model.GameGenerating,
		Intent: "Make a gentle game from this story",
	}
	st.CreateGame(game)
	runner := NewRunner(st)
	job := runner.CreateAndStart(game, nil, game.Intent, nil)
	waitPipelineStatus(t, st, job.ID, model.JobAwaitingConfirm)
	pending, ok := st.GetJob(job.ID)
	if !ok || pending.DesignDoc == nil {
		t.Fatal("missing confirmation design document")
	}
	doc := *pending.DesignDoc
	doc.Title = "Friendly Fallback Quest"
	doc.Elements = nil // creator explicitly confirmed no extracted image anchors
	if _, err := runner.Confirm(game.ID, &doc); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	waitPipelineStatus(t, st, job.ID, model.JobDone)
	published, ok := st.GetGame(game.ID)
	if !ok || published.Status != model.GameReady || published.Manifest == nil || published.GameIR == nil {
		t.Fatalf("empty questionnaire failed to publish: %+v", published)
	}

	const uri = "sprites/el_1.png"
	var referencedByIR bool
	for _, asset := range published.GameIR.Assets {
		if asset.ID == "player" && asset.URI == uri {
			referencedByIR = true
		}
	}
	if !referencedByIR {
		t.Fatal("generated IR must reference the actual fallback player PNG")
	}
	var presentInManifest bool
	for _, sprite := range published.Manifest.Assets.Sprites {
		if sprite.ID == "player" && sprite.Src == uri {
			presentInManifest = true
		}
	}
	if !presentInManifest {
		t.Fatal("fallback player missing from published asset manifest")
	}
	png, mime, ok := st.GetArtifact(game.ID, uri)
	if !ok || mime != "image/png" || !bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatalf("published fallback image missing, malformed or wrong MIME: exists=%v mime=%q bytes=%d", ok, mime, len(png))
	}
}
