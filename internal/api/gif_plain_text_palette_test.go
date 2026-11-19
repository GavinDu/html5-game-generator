package api

import (
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// Each uploaded GIF is a standalone data stream. Plain Text uses the GLOBAL
// color table, not a local table on the following image.
type plainTextColorCase struct {
	name                        string
	globalEntries, localEntries int
	foreground, background      byte
	wantOK                      bool
}

func plainTextColorCases() []plainTextColorCase {
	return []plainTextColorCase{
		{name: "global_two_both_valid", globalEntries: 2, foreground: 0, background: 1, wantOK: true},
		{name: "global_four_both_valid", globalEntries: 4, foreground: 3, background: 2, wantOK: true},
		{name: "global_two_bad_foreground", globalEntries: 2, foreground: 2, background: 1},
		{name: "global_two_bad_background", globalEntries: 2, foreground: 1, background: 2},
		{name: "local_four_does_not_extend_global_two", globalEntries: 2, localEntries: 4,
			foreground: 3, background: 1},
		{name: "local_palette_cannot_replace_missing_global", localEntries: 2,
			foreground: 0, background: 1},
	}
}

func plainTextPaletteFixture(tc plainTextColorCase) []byte {
	encoded := tinyPaletteIndexedGIF(1, tc.globalEntries, tc.localEntries)
	text := gifPlainTextGridBlock(12)
	// Extension introducer + label + size precede the fixed 12-byte header.
	text[3+10] = tc.foreground
	text[3+11] = tc.background
	return graftGIFBlocks(encoded, tc.globalEntries, text)
}

func TestGIFPlainTextColorsRequireGlobalPalette(t *testing.T) {
	for _, tc := range plainTextColorCases() {
		t.Run(tc.name, func(t *testing.T) {
			encoded := plainTextPaletteFixture(tc)
			if err := gifFrameMetadataBudget(encoded); err != nil {
				t.Fatalf("bounded GIF fixture has invalid metadata: %v", err)
			}
			err := gifLZWFrameBudget(encoded)
			if (err == nil) != tc.wantOK {
				t.Fatalf("plain text palette admitted=%v, wantOK=%v: %v",
					err == nil, tc.wantOK, err)
			}
			_, err = rasterHeaderPixels("image/gif", encoded)
			if (err == nil) != tc.wantOK {
				t.Fatalf("production raster admitted=%v, wantOK=%v: %v",
					err == nil, tc.wantOK, err)
			}
		})
	}
}

func TestGIFPlainTextColorsFailClosedForPackageAndDirectAsset(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	const gameID = "invalid-gif-plain-text-colors"
	const uri = "assets/plain-text-colors.gif"
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
	for _, tc := range plainTextColorCases() {
		t.Run(tc.name, func(t *testing.T) {
			st.PutArtifact(gameID, uri, plainTextPaletteFixture(tc), "image/gif")
			for _, path := range paths {
				resp := getAs(t, srv, path, "")
				if tc.wantOK {
					if resp.Code != http.StatusOK {
						t.Errorf("valid GIF text palette rejected at %s: HTTP %d", path, resp.Code)
					}
					continue
				}
				if resp.Code != http.StatusUnprocessableEntity ||
					resp.Header().Get("Cache-Control") != "private, no-store" {
					t.Errorf("invalid GIF text palette at %s: HTTP %d cache=%q",
						path, resp.Code, resp.Header().Get("Cache-Control"))
				}
			}
		})
	}
}
