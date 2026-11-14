package api

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func tinyRasterPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 12, G: 24, B: 36, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// The signature/IHDR/CRC remain well-formed without allocating the declared
// decoded dimensions. PNG header inspection must refuse this low-byte bomb.
func pngDeclaredDimensions(t *testing.T, width, height uint32) []byte {
	t.Helper()
	data := tinyRasterPNG(t)
	binary.BigEndian.PutUint32(data[16:20], width)
	binary.BigEndian.PutUint32(data[20:24], height)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestRasterHeaderAdmissionRejectsCompressedDimensionBomb(t *testing.T) {
	valid := tinyRasterPNG(t)
	pixels, err := rasterHeaderPixels("image/png", valid)
	if err != nil || pixels != 1 {
		t.Fatalf("valid tiny PNG header rejected: pixels=%d err=%v", pixels, err)
	}
	if _, err := rasterHeaderPixels("image/png", pngDeclaredDimensions(t, 8193, 1)); err == nil {
		t.Fatal("dimension over maxRasterDimension admitted")
	}
	if _, err := rasterHeaderPixels("image/png", pngDeclaredDimensions(t, 8192, 8192)); err == nil {
		t.Fatal("enormous decoded image passed pixel admission")
	}
	if pixels, err = rasterHeaderPixels("image/png", pngDeclaredDimensions(t, 4096, 4096)); err != nil || pixels != maxRasterPixels {
		t.Fatalf("exact decoded-pixel limit should pass header check: pixels=%d err=%v", pixels, err)
	}
	if _, err := rasterHeaderPixels("image/png", valid[:12]); err == nil {
		t.Fatal("truncated raster header admitted")
	}
	if _, err := rasterHeaderPixels("image/jpeg", valid); err == nil {
		t.Fatal("declared MIME spoof against encoded PNG admitted")
	}
	if pixels, err = rasterHeaderPixels("", valid); err != nil || pixels != 1 {
		t.Fatalf("legacy absent MIME must still sniff valid header and enforce pixels: %d %v", pixels, err)
	}
	if _, err := rasterHeaderPixels("", valid[:12]); err == nil {
		t.Fatal("legacy absent MIME must reject truncated raster bytes")
	}
	for _, mime := range []string{"image/webp", "image/svg+xml", "application/octet-stream", "image/png; charset=utf-8"} {
		if _, err := rasterHeaderPixels(mime, valid); err == nil {
			t.Fatalf("unsupported declared image MIME %q bypassed raster admission", mime)
		}
	}
}

func TestRasterHeaderAdmissionsIncludeJPEGAndGIF(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	if pixels, err := rasterHeaderPixels("image/jpeg", jpg.Bytes()); err != nil || pixels != 1 {
		t.Fatalf("valid JPEG header rejected: %d %v", pixels, err)
	}
	paletted := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	var gifBytes bytes.Buffer
	if err := gif.Encode(&gifBytes, paletted, nil); err != nil {
		t.Fatal(err)
	}
	if pixels, err := rasterHeaderPixels("image/gif", gifBytes.Bytes()); err != nil || pixels != 1 {
		t.Fatalf("valid GIF header rejected: %d %v", pixels, err)
	}
}

func TestRasterAggregatePixelCeiling(t *testing.T) {
	if !packageRasterAggregateAllows(maxPackageRasterPixels-maxRasterPixels, maxRasterPixels) {
		t.Fatal("exact aggregate decoded raster budget must be admitted")
	}
	if packageRasterAggregateAllows(maxPackageRasterPixels-maxRasterPixels+1, maxRasterPixels) {
		t.Fatal("over aggregate decoded raster budget admitted")
	}
	if packageRasterAggregateAllows(0, maxRasterPixels+1) {
		t.Fatal("per-image decoded raster budget bypassed")
	}
	if packageRasterAggregateAllows(maxPackageRasterPixels+1, 0) {
		t.Fatal("already-overbudget package admitted")
	}
}

func TestPublishedPackageAndAssetFailClosedOnRasterHeaderBudget(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "raster-header-budget-game")
	game.ModerationStatus = "approved"
	const uri = "assets/raster-header-budget.png"
	const id = "raster-header-budget"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: id, Kind: "image", URI: uri, MIMEType: "image/png",
	})
	st.UpdateGame(game)
	st.PutArtifact(game.ID, uri, tinyRasterPNG(t), "image/png")
	if got := getAs(t, srv, "/v1/games/raster-header-budget-game/package", ""); got.Code != http.StatusOK {
		t.Fatalf("valid package rejected: %d %s", got.Code, got.Body.String())
	}
	if got := getAs(t, srv, "/v1/games/raster-header-budget-game/assets/raster-header-budget", ""); got.Code != http.StatusOK {
		t.Fatalf("valid game asset rejected: %d %s", got.Code, got.Body.String())
	}
	st.PutArtifact(game.ID, uri, pngDeclaredDimensions(t, 8192, 8192), "image/png")
	for _, route := range []string{
		"/v1/games/raster-header-budget-game/package",
		"/v1/games/raster-header-budget-game/assets/raster-header-budget",
	} {
		got := getAs(t, srv, route, "")
		if got.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: oversized raster header returned %d instead of 422: %s", route, got.Code, got.Body.String())
		}
		if got.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s: rejected raster should never be cached", route)
		}
	}
}
