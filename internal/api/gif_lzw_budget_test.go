package api

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

func TestBoundedGIFLZWRejectsUnderflowAndCorruptCodes(t *testing.T) {
	for _, frames := range []int{1, 2, maxGIFFrames} {
		valid := encodeTinyAnimatedGIF(t, frames)
		if err := gifLZWFrameBudget(valid); err != nil {
			t.Fatalf("valid %d frame GIF rejected by bounded LZW: %v", frames, err)
		}
		if _, err := rasterHeaderPixels("image/gif", valid); err != nil {
			t.Fatalf("valid %d frame GIF rejected by production raster gate: %v", frames, err)
		}
	}

	// This metadata-only fixture declares 2x2 pixels but emits only one
	// valid index. Prior #410 only checks the container and deliberately
	// admits this synthetic metadata probe; the production LZW gate must not.
	underflow := syntheticGIFFrameHeaders(2, 2, 1)
	if err := gifFrameMetadataBudget(underflow); err != nil {
		t.Fatalf("syntactic GIF metadata unexpectedly invalid: %v", err)
	}
	if err := gifLZWFrameBudget(underflow); err == nil {
		t.Fatal("GIF frame with early end code admitted")
	}
	if _, err := rasterHeaderPixels("image/gif", underflow); err == nil {
		t.Fatal("production raster gate admitted short decoded frame")
	}

	invalidCode := syntheticGIFFrameHeaders(1, 1, 1)
	invalidCode[len(invalidCode)-4] = 0xff
	invalidCode[len(invalidCode)-3] = 0xff
	if err := gifFrameMetadataBudget(invalidCode); err != nil {
		t.Fatalf("corrupt LZW should remain a metadata-valid GIF: %v", err)
	}
	if err := gifLZWFrameBudget(invalidCode); err == nil {
		t.Fatal("invalid GIF LZW dictionary codes admitted")
	}
}

func TestBoundedGIFLZWRejectsDecodedPixelOverflow(t *testing.T) {
	// Encode real two-by-two pixels, then shrink only metadata to one-by-one.
	// The GIF container remains valid, but LZW still emits four indexes.
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	var out bytes.Buffer
	if err := gif.Encode(&out, frame, nil); err != nil {
		t.Fatalf("encode 2x2 fixture: %v", err)
	}
	oversized := append([]byte(nil), out.Bytes()...)
	binary.LittleEndian.PutUint16(oversized[6:8], 1)
	binary.LittleEndian.PutUint16(oversized[8:10], 1)
	cursor := 13
	if oversized[10]&0x80 != 0 {
		cursor += 3 * (1 << (int(oversized[10]&0x07) + 1))
	}
	for cursor < len(oversized) && oversized[cursor] == 0x21 {
		cursor += 2
		var err error
		cursor, err = skipGIFDataSubblocks(oversized, cursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if cursor >= len(oversized) || oversized[cursor] != 0x2c {
		t.Fatal("fixture image descriptor not found")
	}
	binary.LittleEndian.PutUint16(oversized[cursor+5:cursor+7], 1)
	binary.LittleEndian.PutUint16(oversized[cursor+7:cursor+9], 1)
	if err := gifFrameMetadataBudget(oversized); err != nil {
		t.Fatalf("overflow fixture must remain metadata-valid: %v", err)
	}
	if err := gifLZWFrameBudget(oversized); err == nil {
		t.Fatal("GIF frame emitting four pixels against declared one admitted")
	}
}

func TestPublicGamePackageAndDirectAssetRejectBadGIFLZW(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "gif-invalid-lzw-game")
	game.ModerationStatus = "approved"
	const uri = "assets/bad-animation.gif"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "bad-animation", Kind: "image", URI: uri, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	paths := []string{
		"/v1/games/gif-invalid-lzw-game/package",
		"/v1/games/gif-invalid-lzw-game/assets/bad-animation",
	}
	st.PutArtifact(game.ID, uri, encodeTinyAnimatedGIF(t, 2), "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("valid animated GIF unexpectedly blocked: %s HTTP %d", path, resp.Code)
		}
	}
	st.PutArtifact(game.ID, uri, syntheticGIFFrameHeaders(2, 2, 1), "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusUnprocessableEntity ||
			resp.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("invalid GIF LZW not denied for %s: HTTP %d cache=%q",
				path, resp.Code, resp.Header().Get("Cache-Control"))
		}
	}
}
