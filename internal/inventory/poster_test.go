package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

func posterFixture(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	ir := []byte(`{"game":{"id":"fixture"}}`)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 8))); err != nil {
		t.Fatal(err)
	}
	poster := buf.Bytes()
	hash := func(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
	meta, err := json.Marshal(shippedPoster{Version: 1, IRSHA256: hash(ir), PosterSHA256: hash(poster)})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"preview-fixture.json": meta, "game-ir.json": ir, "preview-poster.png.b64": []byte(base64.StdEncoding.EncodeToString(poster))} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir, poster
}

func TestShippedPosterBindsToIRAndVersion(t *testing.T) {
	dir, poster := posterFixture(t)
	st := store.NewMemoryStore()
	game := &model.Game{ID: "fixture"}
	if err := seedPoster(st, game, nil, dir, 1); err != nil {
		t.Fatal(err)
	}
	if game.Preview == nil || game.Preview.ClipURL != "" || game.Preview.DurationMS != 0 || game.Preview.Aspect != 0.5 {
		t.Fatalf("invalid poster-only preview: %+v", game.Preview)
	}
	data, mime, ok := st.GetArtifact(game.ID, "preview/v1/poster.png")
	if !ok || mime != "image/png" || !bytes.Equal(data, poster) {
		t.Fatal("poster was not retained under the scoped preview key")
	}
	for _, version := range []int{1, 2} {
		if version == 1 {
			if err := os.WriteFile(filepath.Join(dir, "game-ir.json"), []byte("changed gameplay"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		candidate := &model.Game{ID: "fixture"}
		if err := seedPoster(st, candidate, nil, dir, version); err != nil {
			t.Fatal(err)
		}
		if candidate.Preview != nil {
			t.Fatal("stale IR/version must not advertise a poster")
		}
	}
}

func TestShippedPosterRejectsCorruptBytes(t *testing.T) {
	dir, _ := posterFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "preview-poster.png.b64"), []byte(base64.StdEncoding.EncodeToString([]byte("not a PNG"))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := seedPoster(store.NewMemoryStore(), &model.Game{ID: "fixture"}, nil, dir, 1); err == nil {
		t.Fatal("corrupt poster must not load")
	}
}

func TestInventoryRestartRetainsCurrentCreatorPreview(t *testing.T) {
	doc := &gameir.Document{Game: gameir.Game{ID: "fixture"}}
	previous := &model.Game{ID: "fixture", GameIR: doc, Preview: &model.PreviewMetadata{Version: 1, ClipURL: "/approved-clip.mp4", Status: "ready"}}
	game := &model.Game{ID: "fixture", GameIR: doc}
	if err := seedPoster(store.NewMemoryStore(), game, previous, t.TempDir(), 1); err != nil {
		t.Fatal(err)
	}
	if game.Preview == nil || game.Preview.ClipURL != previous.Preview.ClipURL {
		t.Fatal("inventory restart discarded a matching creator preview")
	}
	if game.Preview == previous.Preview {
		t.Fatal("preview metadata must be copied")
	}
}

func TestAllShippedGamesHaveVersionBoundPosters(t *testing.T) {
	st := store.NewMemoryStore()
	if err := Load(st, filepath.Join("..", "..", "inventory")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"sample-rainbow-bear", "escape-from-injection", "penguin-heartlight-rescue"} {
		game, ok := st.GetGame(id)
		if !ok || game.Preview == nil || game.Preview.PosterURL == "" || game.Preview.ClipURL != "" {
			t.Fatalf("%s has no honest poster fallback", id)
		}
	}
	// Ribbon has new six-hole gameplay. Its previous approved poster is
	// deliberately invalidated by the IR hash until fresh screenshot review
	// produces a version-bound replacement. Never advertise the old art as
	// evidence of the changed game.
	ribbon, ok := st.GetGame("sample-ribbon-sky")
	if !ok || ribbon.Preview != nil {
		t.Fatal("changed Ribbon gameplay must invalidate the stale approved poster")
	}
}
