package api

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/pipeline"
	"doodlequest/internal/store"
)

// A real production Server routing call (with in-memory test data) must serve
// authorized GamePackage media bytes directly, not redirect a bearer-bearing
// browser Fetch to another origin. This is NOT an attested deployed CDN test.
func TestPublicPackageAssetDeliveryDoesNotRedirect(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(st, pipeline.NewRunner(st))
	game := seedReadyGame(st, "private-asset-redirect-contract")
	game.ModerationStatus = "approved"

	const imageURI = "assets/static.gif"
	const audioURI = "assets/safe-sound.wav"
	game.GameIR.Assets = append(game.GameIR.Assets,
		gameir.Asset{ID: "static-gif", Kind: "image", URI: imageURI, MIMEType: "image/gif"},
		gameir.Asset{ID: "safe-sound", Kind: "audio", URI: audioURI, MIMEType: "audio/wav"},
	)
	st.UpdateGame(game)
	st.PutArtifact(game.ID, imageURI, encodeTinyAnimatedGIF(t, 1), "image/gif")
	st.PutArtifact(game.ID, audioURI, tinyWAVForAssetRedirectContract(), "audio/wav")

	for _, tc := range []struct {
		name string
		path string
	}{
		{"package", "/v1/games/private-asset-redirect-contract/package"},
		{"image", "/v1/games/private-asset-redirect-contract/assets/static-gif"},
		{"audio", "/v1/games/private-asset-redirect-contract/assets/safe-sound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := getAs(t, srv, tc.path, "")
			if response.Code != http.StatusOK {
				t.Fatalf("published package media must be served directly: %s HTTP %d, response %q",
					tc.path, response.Code, response.Body.String())
			}
			if redirect := response.Header().Get("Location"); redirect != "" {
				t.Fatalf("published package media must not redirect: %s Location %q", tc.path, redirect)
			}
			if response.Body.Len() == 0 {
				t.Fatalf("published package media cannot be empty: %s", tc.path)
			}
		})
	}
}

// Minimal legal, finite PCM WAV. No third-party network or decoder is used;
// the image/audio artifact handler is the source under test.
func tinyWAVForAssetRedirectContract() []byte {
	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(44))
	out.WriteString("WAVEfmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint32(8000))
	_ = binary.Write(&out, binary.LittleEndian, uint32(8000))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(8))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, uint32(8))
	out.Write(bytes.Repeat([]byte{128}, 8))
	return out.Bytes()
}
