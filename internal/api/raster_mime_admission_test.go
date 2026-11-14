package api

import (
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestPublishedImageRequiresAdmittedMIMEAtPackageAndDirectAssetEndpoints(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "mime-budget-game")
	game.ModerationStatus = "approved"
	const uri = "assets/mime-budget.png"
	const id = "mime-budget"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: id, Kind: "image", URI: uri, MIMEType: "image/png",
	})
	st.UpdateGame(game)
	// Real, valid PNG so only the declared metadata controls rejection.
	st.PutArtifact(game.ID, uri, tinyRasterPNG(t), "image/png")
	paths := []string{
		"/v1/games/mime-budget-game/package",
		"/v1/games/mime-budget-game/assets/mime-budget",
	}
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("valid declared PNG %s: HTTP %d %s", path, resp.Code, resp.Body.String())
		}
	}

	// A v1 published Game IR historically omitted mimeType. Its actual
	// PNG bytes must still be sniffed and admitted under the SAME budget.
	legacy, ok := st.GetGame(game.ID)
	if !ok {
		t.Fatal("seeded legacy game missing")
	}
	for i := range legacy.GameIR.Assets {
		if legacy.GameIR.Assets[i].ID == id {
			legacy.GameIR.Assets[i].MIMEType = ""
		}
	}
	st.UpdateGame(legacy)
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("legacy MIME-less valid PNG %s: HTTP %d %s", path, resp.Code, resp.Body.String())
		}
	}

	for _, mime := range []string{"image/webp", "image/svg+xml", "application/octet-stream"} {
		t.Run("declared-"+mime, func(t *testing.T) {
			existing, ok := st.GetGame(game.ID)
			if !ok {
				t.Fatal("seeded game missing")
			}
			for index := range existing.GameIR.Assets {
				if existing.GameIR.Assets[index].ID == id {
					existing.GameIR.Assets[index].MIMEType = mime
				}
			}
			st.UpdateGame(existing)
			for _, path := range paths {
				resp := getAs(t, srv, path, "")
				if resp.Code != http.StatusUnprocessableEntity {
					t.Fatalf("image MIME %q %s: HTTP %d, want 422: %s",
						mime, path, resp.Code, resp.Body.String())
				}
				if got := resp.Header().Get("Cache-Control"); got != "private, no-store" {
					t.Fatalf("rejected image MIME %q must never be shared cached: %q", mime, got)
				}
			}
		})
	}
}
