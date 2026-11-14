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

func encodeTinyAnimatedGIF(t *testing.T, frames int) []byte {
	t.Helper()
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	anim := &gif.GIF{
		Image: make([]*image.Paletted, frames),
		Delay: make([]int, frames),
	}
	for i := range anim.Image {
		anim.Image[i] = frame
		anim.Delay[i] = 2
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, anim); err != nil {
		t.Fatalf("encode animation: %v", err)
	}
	return out.Bytes()
}

// The image descriptor declares a huge *decoded* frame but carries only a
// tiny LZW subblock. Header-only admission must not allocate it. Four 4096²
// frames exceed the 64M aggregate limit even though the GIF is < 100 bytes.
func syntheticGIFFrameHeaders(width, height uint16, frames int) []byte {
	data := append([]byte("GIF89a"), 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint16(data[6:8], width)
	binary.LittleEndian.PutUint16(data[8:10], height)
	for range frames {
		data = append(data,
			0x2c,       // image separator
			0, 0, 0, 0, // left and top
			0, 0, 0, 0, // width and height
			0x80, // local 2-color table
			0, 0, 0, 255, 255, 255,
			0x02,             // minimum LZW code size
			0x02, 0x4c, 0x01, // one tiny compressed subblock
			0, // subblock terminator
		)
		binary.LittleEndian.PutUint16(data[len(data)-16:len(data)-14], width)
		binary.LittleEndian.PutUint16(data[len(data)-14:len(data)-12], height)
	}
	return append(data, 0x3b)
}

func TestGIFContainerMetadataAdmitsSmallAnimationAndRejectsFrameBomb(t *testing.T) {
	for _, count := range []int{1, 2, maxGIFFrames} {
		data := encodeTinyAnimatedGIF(t, count)
		if err := gifFrameMetadataBudget(data); err != nil {
			t.Fatalf("%d valid 1px GIF frames rejected: %v", count, err)
		}
		if pixels, err := rasterHeaderPixels("image/gif", data); err != nil || pixels != 1 {
			t.Fatalf("%d valid GIF frames rejected via production raster gate: pixels=%d err=%v", count, pixels, err)
		}
	}
	tooMany := encodeTinyAnimatedGIF(t, maxGIFFrames+1)
	if err := gifFrameMetadataBudget(tooMany); err == nil {
		t.Fatal("GIF animation with over 128 frames admitted")
	}
	if _, err := rasterHeaderPixels("image/gif", tooMany); err == nil {
		t.Fatal("production header admission ignored animation frame count")
	}

	// Tiny encoded bytes with four declared maximum-size frames must be
	// rejected before any image.DecodeAll/rgba allocation.
	oversized := syntheticGIFFrameHeaders(4096, 4096, 4)
	if len(oversized) > 128 {
		t.Fatal("test fixture unexpectedly expensive")
	}
	if err := gifFrameMetadataBudget(oversized); err == nil {
		t.Fatal("aggregate decoded GIF frame pixels exceeded 64M limit")
	}
	if err := gifFrameMetadataBudget(syntheticGIFFrameHeaders(4096, 4096, 3)); err != nil {
		t.Fatalf("valid aggregate decoded-frame limit exceeded too early: %v", err)
	}
	if err := gifFrameMetadataBudget(syntheticGIFFrameHeaders(2, 2, 1)); err != nil {
		t.Fatalf("synthetic bounded image descriptor rejected: %v", err)
	}
}

func TestGIFContainerMetadataRejectsTruncationAndTrailingContent(t *testing.T) {
	valid := encodeTinyAnimatedGIF(t, 2)
	for _, corrupted := range [][]byte{
		valid[:12],
		valid[:len(valid)-1],                     // missing terminator
		append(append([]byte(nil), valid...), 0), // trailing garbage
		append(append([]byte(nil), valid[:len(valid)-2]...), 0xff, 0x3b), // broken final LZW subblock
	} {
		if err := gifFrameMetadataBudget(corrupted); err == nil {
			t.Fatalf("corrupt GIF container admitted: %x", corrupted[:min(len(corrupted), 16)])
		}
	}
}

func TestGamePackageAndDirectAssetRejectExcessAnimatedGIFFrames(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "gif-frame-budget-game")
	game.ModerationStatus = "approved"
	const uri = "assets/frame-budget.gif"
	const id = "frame-budget"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: id, Kind: "image", URI: uri, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	st.PutArtifact(game.ID, uri, encodeTinyAnimatedGIF(t, 2), "image/gif")
	paths := []string{
		"/v1/games/gif-frame-budget-game/package",
		"/v1/games/gif-frame-budget-game/assets/frame-budget",
	}
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("legal tiny animated GIF %s HTTP %d %s", path, resp.Code, resp.Body.String())
		}
	}
	st.PutArtifact(game.ID, uri, encodeTinyAnimatedGIF(t, maxGIFFrames+1), "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusUnprocessableEntity ||
			resp.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("frame-bomb GIF %s not fail-closed: HTTP %d cache=%q body=%s",
				path, resp.Code, resp.Header().Get("Cache-Control"), resp.Body.String())
		}
	}
}
