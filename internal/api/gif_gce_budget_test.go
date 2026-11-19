package api

import (
	"fmt"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// graftGIFBlocks inserts real GIF extension blocks before the first image
// descriptor. This preserves a valid, LZW-compressed image payload and lets
// the test change GCE semantics without relying on DecodeAll.
func graftGIFBlocks(encoded []byte, globalPaletteEntries int, blocks ...[]byte) []byte {
	start := 13 + 3*globalPaletteEntries
	out := append([]byte{}, encoded[:start]...)
	for _, block := range blocks {
		out = append(out, block...)
	}
	return append(out, encoded[start:]...)
}

func tinyGIFGraphicControl(packed, index byte) []byte {
	return []byte{0x21, 0xf9, 4, packed, 0, 0, index, 0}
}

func TestGIFGraphicControlTransparencyBoundsArePerFrame(t *testing.T) {
	for _, tc := range []struct {
		name        string
		index       byte
		global      int
		local       int
		packed      byte
		transparent byte
		wantOK      bool
	}{
		{name: "global_two_valid", index: 1, global: 2, packed: 1, transparent: 1, wantOK: true},
		{name: "global_two_invalid_transparency", index: 1, global: 2, packed: 1, transparent: 2},
		{name: "local_four_over_global_two", index: 3, global: 2, local: 4, packed: 1, transparent: 3, wantOK: true},
		{name: "local_two_over_global_four", index: 1, global: 4, local: 2, packed: 1, transparent: 3},
		{name: "no_transparency_flag_ignores_index", index: 1, global: 2, packed: 0, transparent: 255, wantOK: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gif := graftGIFBlocks(tinyPaletteIndexedGIF(tc.index, tc.global, tc.local),
				tc.global, tinyGIFGraphicControl(tc.packed, tc.transparent))
			if err := gifFrameMetadataBudget(gif); err != nil {
				t.Fatalf("fixture must have valid GIF metadata: %v", err)
			}
			err := gifLZWFrameBudget(gif)
			if (err == nil) != tc.wantOK {
				t.Fatalf("GCE per-frame palette admission=%v, wantOK=%v: %v", err == nil, tc.wantOK, err)
			}
		})
	}
}

func TestGIFGraphicControlExtensionLifetimeAndMalformedSubblocks(t *testing.T) {
	base := tinyPaletteIndexedGIF(1, 2, 0)
	comment := []byte{0x21, 0xfe, 1, 'x', 0}
	// A comment is non-graphic: the GCE still belongs to the next image.
	withComment := graftGIFBlocks(base, 2, tinyGIFGraphicControl(1, 2), comment)
	if err := gifFrameMetadataBudget(withComment); err != nil {
		t.Fatalf("metadata-valid comment fixture: %v", err)
	}
	if err := gifLZWFrameBudget(withComment); err == nil {
		t.Fatal("comment extension incorrectly consumed invalid pending transparency")
	}
	withValidComment := graftGIFBlocks(base, 2, tinyGIFGraphicControl(1, 1), comment)
	if err := gifLZWFrameBudget(withValidComment); err != nil {
		t.Fatalf("comment incorrectly invalidated valid GCE: %v", err)
	}
	// Plain Text Extension is a graphic-rendering block and consumes GCE.
	plainText := []byte{0x21, 0x01, 12, 0, 0, 0, 0, 1, 0, 1, 0, 8, 8, 0, 0, 0}
	withPlainText := graftGIFBlocks(base, 2, tinyGIFGraphicControl(1, 2), plainText)
	if err := gifFrameMetadataBudget(withPlainText); err != nil {
		t.Fatalf("metadata-valid plain-text fixture: %v", err)
	}
	if err := gifLZWFrameBudget(withPlainText); err != nil {
		t.Fatalf("plain text must consume prior GCE: %v", err)
	}
	for name, bad := range map[string][]byte{
		"short_gce":          {0x21, 0xf9, 3, 1, 0, 0, 0},
		"extra_gce_subblock": {0x21, 0xf9, 4, 1, 0, 0, 1, 1, 0xaa, 0},
	} {
		t.Run(name, func(t *testing.T) {
			gif := graftGIFBlocks(base, 2, bad)
			if err := gifFrameMetadataBudget(gif); err != nil {
				t.Fatalf("fixture must be metadata-valid to exercise GCE parser: %v", err)
			}
			if err := gifLZWFrameBudget(gif); err == nil {
				t.Fatal("malformed GIF GCE must fail closed")
			}
		})
	}
	duplicate := graftGIFBlocks(base, 2, tinyGIFGraphicControl(1, 1), tinyGIFGraphicControl(1, 1))
	if err := gifLZWFrameBudget(duplicate); err == nil {
		t.Fatal("duplicate unconsumed GIF GCE must fail closed")
	}
	// A control extension after the last image has no following graphic.
	last := append([]byte{}, base[:len(base)-1]...)
	last = append(last, tinyGIFGraphicControl(1, 1)...)
	last = append(last, 0x3b)
	if err := gifLZWFrameBudget(last); err == nil {
		t.Fatal("dangling GCE at trailer must fail closed")
	}
}

func TestGCEOutOfRangeTransparencyFailsPackageAndDirectAsset(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	const gameID = "invalid-gif-gce-game"
	const uri = "assets/gce.gif"
	game := seedReadyGame(st, gameID)
	game.ModerationStatus = "approved"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "gce-test", Kind: "image", URI: uri, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	paths := []string{
		"/v1/games/" + gameID + "/package",
		"/v1/games/" + gameID + "/assets/gce-test",
	}
	valid := graftGIFBlocks(tinyPaletteIndexedGIF(1, 2, 0), 2, tinyGIFGraphicControl(1, 1))
	st.PutArtifact(gameID, uri, valid, "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("valid GCE denied for %s: %d %s", path, resp.Code, resp.Body.String())
		}
	}
	invalid := graftGIFBlocks(tinyPaletteIndexedGIF(1, 2, 0), 2, tinyGIFGraphicControl(1, 2))
	st.PutArtifact(gameID, uri, invalid, "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusUnprocessableEntity ||
			resp.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("invalid GCE must fail closed at %s: HTTP %d cache=%q",
				path, resp.Code, resp.Header().Get("Cache-Control"))
		}
	}
}
// gifPlainTextGridBlock constructs a GIF89a Plain Text Extension with a
// deliberately selectable first header-subblock size. A valid one contains
// exactly twelve text-grid bytes; the surrounding GIF/LZW image stays real.
func gifPlainTextGridBlock(headerSize int) []byte {
	header := []byte{0, 0, 0, 0, 1, 0, 1, 0, 8, 8, 0, 1}
	block := []byte{0x21, 0x01, byte(headerSize)}
	if headerSize == 0 {
		// A zero-size subblock is already the end-of-data sentinel.
		return block
	}
	if headerSize <= len(header) {
		block = append(block, header[:headerSize]...)
	} else {
		block = append(block, header...)
		block = append(block, make([]byte, headerSize-len(header))...)
	}
	return append(block, 0) // terminates the plain-text data subblocks
}

func TestGIFPlainTextHeaderRequiresExactly12Bytes(t *testing.T) {
	base := tinyPaletteIndexedGIF(1, 2, 0)
	for _, size := range []int{12, 0, 11, 13} {
		t.Run(fmt.Sprintf("header_bytes_%d", size), func(t *testing.T) {
			encoded := graftGIFBlocks(
				base, 2, tinyGIFGraphicControl(1, 1),
				gifPlainTextGridBlock(size),
			)
			// The original metadata gate intentionally admits any bounded
			// extension block. This second pass must inspect its grammar.
			if err := gifFrameMetadataBudget(encoded); err != nil {
				t.Fatalf("bounded plain-text fixture must be metadata-valid: %v", err)
			}
			err := gifLZWFrameBudget(encoded)
			if (err == nil) != (size == 12) {
				t.Fatalf("plain text header size %d admitted=%v, want valid=%v: %v",
					size, err == nil, size == 12, err)
			}
			_, contractErr := rasterHeaderPixels("image/gif", encoded)
			if (contractErr == nil) != (size == 12) {
				t.Fatalf("production image admission for header %d: %v", size, contractErr)
			}
		})
	}
}

func TestMalformedGIFPlainTextHeaderFailsPackageAndDirectAsset(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	const gameID = "invalid-gif-plaintext-game"
	const uri = "assets/text-grid.gif"
	game := seedReadyGame(st, gameID)
	game.ModerationStatus = "approved"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "text-grid", Kind: "image", URI: uri, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	paths := []string{
		"/v1/games/" + gameID + "/package",
		"/v1/games/" + gameID + "/assets/text-grid",
	}
	base := tinyPaletteIndexedGIF(1, 2, 0)
	for _, size := range []int{12, 0, 11, 13} {
		encoded := graftGIFBlocks(base, 2, gifPlainTextGridBlock(size))
		st.PutArtifact(gameID, uri, encoded, "image/gif")
		for _, path := range paths {
			resp := getAs(t, srv, path, "")
			if size == 12 {
				if resp.Code != http.StatusOK {
					t.Errorf("valid 12-byte grid denied at %s: HTTP %d", path, resp.Code)
				}
				continue
			}
			if resp.Code != http.StatusUnprocessableEntity ||
				resp.Header().Get("Cache-Control") != "private, no-store" {
				t.Errorf("malformed grid size %d must fail closed at %s: HTTP %d cache=%q",
					size, path, resp.Code, resp.Header().Get("Cache-Control"))
			}
		}
	}
}
