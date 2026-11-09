package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"doodlequest/internal/gameir"
	"doodlequest/internal/store"
)

func TestValidateGeneratedAssetsChecksPersistedBytesBeforePublish(t *testing.T) {
	const gameID = "asset-gate-game"
	const uri = "sprites/hero.png"
	validBytes := []byte("rendered-asset-bytes")
	sum := sha256.Sum256(validBytes)
	hash := hex.EncodeToString(sum[:])
	cases := []struct {
		name         string
		uri          string
		bytes        []byte
		storedMime   string
		declaredMime string
		declaredHash string
		want         string
	}{
		{name: "valid matching artifact", uri: uri, bytes: validBytes, storedMime: "image/png",
			declaredMime: "image/png", declaredHash: strings.ToUpper(hash)},
		{name: "missing artifact", uri: uri, declaredMime: "image/png", want: "has no stored bytes"},
		{name: "zero byte asset", uri: uri, bytes: []byte{}, storedMime: "image/png",
			declaredMime: "image/png", want: "has no stored bytes"},
		{name: "wrong mime", uri: uri, bytes: validBytes, storedMime: "text/plain",
			declaredMime: "image/png", want: "has MIME"},
		{name: "checksum mismatch", uri: uri, bytes: validBytes, storedMime: "image/png",
			declaredMime: "image/png", declaredHash: strings.Repeat("0", 64), want: "SHA-256 mismatch"},
		{name: "missing URI", uri: "", declaredMime: "image/png", want: "has no URI"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore()
			if tc.bytes != nil {
				st.PutArtifact(gameID, tc.uri, tc.bytes, tc.storedMime)
			}
			ir := &gameir.Document{Assets: []gameir.Asset{{
				ID: "hero", URI: tc.uri, MIMEType: tc.declaredMime,
				SHA256: tc.declaredHash,
			}}}
			err := validateGeneratedAssets(st, gameID, ir)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("valid asset failed integrity check: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateGeneratedAssets() = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateGeneratedAssetsRejectsAbsentCanonicalIR(t *testing.T) {
	if err := validateGeneratedAssets(store.NewMemoryStore(), "g", nil); err == nil ||
		!strings.Contains(err.Error(), "missing canonical Game IR") {
		t.Fatalf("nil Game IR should fail validation: %v", err)
	}
}
