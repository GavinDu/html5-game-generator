package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// Shipped posters are reviewed CI screenshots, bound to the exact authored IR.
// They do not use the user-upload path or grant consent to publish source media.
type shippedPoster struct {
	Version      int    `json:"version"`
	IRSHA256     string `json:"ir_sha256"`
	PosterSHA256 string `json:"poster_sha256"`
}

func seedPoster(st store.Store, game *model.Game, previous *model.Game, dir string, version int) error {
	// Restarting the inventory must retain a creator's current preview when
	// both the published version and its canonical gameplay are unchanged.
	if previous != nil && previous.Preview != nil && previous.Preview.Version == version && previous.GameIR != nil {
		oldIR, _ := json.Marshal(previous.GameIR)
		newIR, _ := json.Marshal(game.GameIR)
		if bytes.Equal(oldIR, newIR) {
			copy := *previous.Preview
			game.Preview = &copy
			return nil
		}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "preview-fixture.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var fixture shippedPoster
	if err := json.Unmarshal(raw, &fixture); err != nil {
		return fmt.Errorf("invalid shipped preview metadata: %w", err)
	}
	ir, err := os.ReadFile(filepath.Join(dir, "game-ir.json"))
	if err != nil {
		return err
	}
	irHash := sha256.Sum256(ir)
	if fixture.Version != version || fixture.IRSHA256 != hex.EncodeToString(irHash[:]) {
		// A gameplay update invalidates old screenshots. Play still works;
		// a new reviewed poster is required before it can appear in feeds.
		return nil
	}
	encoded, err := os.ReadFile(filepath.Join(dir, "preview-poster.png.b64"))
	if err != nil {
		return err
	}
	if len(encoded) > 3<<20 {
		return fmt.Errorf("shipped preview exceeds the encoded size budget")
	}
	poster, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		return fmt.Errorf("invalid shipped preview base64: %w", err)
	}
	posterHash := sha256.Sum256(poster)
	if len(poster) > 2<<20 || fixture.PosterSHA256 != hex.EncodeToString(posterHash[:]) {
		return fmt.Errorf("shipped preview size or integrity mismatch")
	}
	config, err := png.DecodeConfig(bytes.NewReader(poster))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
		return fmt.Errorf("invalid shipped preview PNG dimensions")
	}
	st.PutArtifact(game.ID, fmt.Sprintf("preview/v%d/poster.png", version), poster, "image/png")
	game.Preview = &model.PreviewMetadata{
		Version:   version,
		PosterURL: fmt.Sprintf("/v1/games/%s/previews/%d/poster.png", url.PathEscape(game.ID), version),
		Aspect:    float64(config.Width) / float64(config.Height),
		// Existing poster-only fallback contract: no clip and no fake duration.
		Status: "failed",
	}
	return nil
}
