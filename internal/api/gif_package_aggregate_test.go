package api

import (
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestAggregateGIFFramePixelsCountAllAnimationFrames(t *testing.T) {
	// Each GIF declares 34 * 1024^2 frame pixels (under 64M), while
	// two GIFs together declare > 64M. Their logical canvases alone
	// total only 2M, so the old package check would admit both.
	animation := syntheticGIFFrameHeaders(1024, 1024, 34)
	perGIF, err := rasterPackageDecodedCost("image/gif", animation)
	if err != nil {
		t.Fatalf("synthetic bounded GIF must pass header-only admission: %v", err)
	}
	if want := uint64(34 * 1024 * 1024); perGIF != want {
		t.Fatalf("GIF package cost must count all frame rectangles: got %d want %d", perGIF, want)
	}
	if !packageDecodedRasterCostAllows(0, perGIF) {
		t.Fatal("one individually valid GIF must fit the package budget")
	}
	if packageDecodedRasterCostAllows(perGIF, perGIF) {
		t.Fatal("two valid GIFs must exceed their shared frame-memory ceiling")
	}
	if gifLegacy, err := rasterPackageDecodedCost("", animation); err != nil || gifLegacy != perGIF {
		t.Fatalf("legacy MIME-less GIF must have identical frame budget: %d %v", gifLegacy, err)
	}
	if pngCost, err := rasterPackageDecodedCost("image/png", tinyRasterPNG(t)); err != nil || pngCost != 1 {
		t.Fatalf("static PNG still counts its canvas pixels: %d %v", pngCost, err)
	}
	if !packageDecodedRasterCostAllows(maxPackageRasterPixels, 0) ||
		packageDecodedRasterCostAllows(maxPackageRasterPixels, 1) ||
		packageDecodedRasterCostAllows(maxPackageRasterPixels+1, 0) {
		t.Fatal("aggregate limit must use exact overflow-safe bounds")
	}
}

func TestPublishedPackageRejectsCombinedGIFFrameMemoryOverBudget(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	const id = "combined-gif-frame-budget-game"
	game := seedReadyGame(st, id)
	game.ModerationStatus = "approved"
	const uriA = "assets/gif-a.gif"
	const uriB = "assets/gif-b.gif"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "gif-a", Kind: "image", URI: uriA, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	animation := syntheticGIFFrameHeaders(1024, 1024, 34)
	st.PutArtifact(id, uriA, animation, "image/gif")
	route := "/v1/games/" + id + "/package"
	if resp := getAs(t, srv, route, ""); resp.Code != http.StatusOK {
		t.Fatalf("single bounded GIF package failed: HTTP %d %s", resp.Code, resp.Body.String())
	}
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "gif-b", Kind: "image", URI: uriB, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	st.PutArtifact(id, uriB, animation, "image/gif")

	resp := getAs(t, srv, route, "")
	if resp.Code != http.StatusUnprocessableEntity ||
		resp.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("multi-GIF frame-bomb package must reject: HTTP %d cache=%q %s",
			resp.Code, resp.Header().Get("Cache-Control"), resp.Body.String())
	}
	// Direct asset delivery remains subject to each GIF's own scanner.
	// The package is the only place where all assets' costs are known.
	for _, asset := range []string{"gif-a", "gif-b"} {
		direct := getAs(t, srv, "/v1/games/"+id+"/assets/"+asset, "")
		if direct.Code != http.StatusOK {
			t.Fatalf("individually admissible GIF %s unexpectedly blocked: %d %s",
				asset, direct.Code, direct.Body.String())
		}
	}
}
