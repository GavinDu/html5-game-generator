package api

import (
	"bytes"
	"compress/lzw"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// tinyPaletteIndexedGIF constructs real, decodable GIF LZW codes while keeping
// the frame's palette metadata independently configurable. Each one-pixel frame
// uses a bounded data subblock, never a decoded bitmap or an image.DecodeAll.
func tinyPaletteIndexedGIF(index byte, globalEntries, localEntries int) []byte {
	palettePacked := func(entries int) byte {
		switch entries {
		case 2:
			return 0x80
		case 4:
			return 0x81
		default:
			return 0
		}
	}
	data := append([]byte("GIF89a"),
		1, 0, 1, 0, palettePacked(globalEntries), 0, 0)
	for i := 0; i < globalEntries; i++ {
		data = append(data, byte(i), byte(i), byte(i))
	}
	data = append(data,
		0x2c, 0, 0, 0, 0, 1, 0, 1, 0, palettePacked(localEntries))
	for i := 0; i < localEntries; i++ {
		data = append(data, byte(i), byte(i), byte(i))
	}
	var out bytes.Buffer
	writer := lzw.NewWriter(&out, lzw.LSB, 2)
	_, _ = writer.Write([]byte{index})
	_ = writer.Close()
	compressed := out.Bytes()
	data = append(data, 2, byte(len(compressed)))
	data = append(data, compressed...)
	return append(data, 0, 0x3b)
}

func TestGIFLZWActivePaletteAdmitsGlobalAndLocalTables(t *testing.T) {
	for _, tt := range []struct {
		name          string
		index         byte
		global, local int
	}{
		{"global_two_index_one", 1, 2, 0},
		{"global_four_index_three", 3, 4, 0},
		{"local_four_over_global_two", 3, 2, 4},
		{"only_local_four", 3, 0, 4},
		{"local_two_over_global_four", 1, 4, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tinyPaletteIndexedGIF(tt.index, tt.global, tt.local)
			if err := gifFrameMetadataBudget(encoded); err != nil {
				t.Fatalf("real LZW frame metadata denied: %v", err)
			}
			if err := gifLZWFrameBudget(encoded); err != nil {
				t.Fatalf("bounded palette index unexpectedly denied: %v", err)
			}
			if _, err := rasterHeaderPixels("image/gif", encoded); err != nil {
				t.Fatalf("production GIF admission rejected valid frame: %v", err)
			}
		})
	}
	for _, n := range []int{1, 2, maxGIFFrames} {
		if err := gifLZWFrameBudget(encodeTinyAnimatedGIF(t, n)); err != nil {
			t.Fatalf("real %d-frame animation no longer accepted: %v", n, err)
		}
	}
}

func TestGIFLZWActivePaletteRejectsOutOfRangeIndicesAndNoColorTable(t *testing.T) {
	for _, tt := range []struct {
		name          string
		index         byte
		global, local int
	}{
		{"global_two_index_two", 2, 2, 0},
		{"global_two_index_three", 3, 2, 0},
		{"local_two_over_global_four", 3, 4, 2},
		{"local_two_no_global", 3, 0, 2},
		{"missing_both_tables", 1, 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tinyPaletteIndexedGIF(tt.index, tt.global, tt.local)
			if err := gifFrameMetadataBudget(encoded); err != nil {
				t.Fatalf("invalid palette fixture must still be metadata-valid: %v", err)
			}
			if err := gifLZWFrameBudget(encoded); err == nil {
				t.Fatal("GIF index outside active color table admitted")
			}
		})
	}
}

func TestPublishedGamePackageAndAssetRejectInvalidGIFPalette(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	const gameID = "invalid-gif-palette-game"
	const uri = "assets/bad-palette.gif"
	game := seedReadyGame(st, gameID)
	game.ModerationStatus = "approved"
	game.GameIR.Assets = append(game.GameIR.Assets, gameir.Asset{
		ID: "palette-test", Kind: "image", URI: uri, MIMEType: "image/gif",
	})
	st.UpdateGame(game)
	paths := []string{
		"/v1/games/" + gameID + "/package",
		"/v1/games/" + gameID + "/assets/palette-test",
	}
	st.PutArtifact(gameID, uri, tinyPaletteIndexedGIF(1, 2, 0), "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusOK {
			t.Fatalf("valid GIF palette denied for %s HTTP %d %s",
				path, resp.Code, resp.Body.String())
		}
	}
	st.PutArtifact(gameID, uri, tinyPaletteIndexedGIF(3, 2, 0), "image/gif")
	for _, path := range paths {
		resp := getAs(t, srv, path, "")
		if resp.Code != http.StatusUnprocessableEntity ||
			resp.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("invalid palette must fail closed on %s: HTTP %d cache=%q",
				path, resp.Code, resp.Header().Get("Cache-Control"))
		}
	}
}
