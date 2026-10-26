package api

import (
	"bytes"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"doodlequest/internal/model"
)

const (
	maxPreviewPosterBytes = 2 << 20
	maxPreviewClipBytes   = 12 << 20
)

type previewMetadataRequest struct {
	Version       int     `json:"version"`
	Status        string  `json:"status"`
	Aspect        float64 `json:"aspect"`
	DurationMS    int     `json:"duration_ms"`
	DominantColor string  `json:"dominant_color"`
}

func publishedGameVersion(game *model.Game) int {
	if game == nil {
		return 0
	}
	if game.PublishedVersion > 0 {
		return game.PublishedVersion
	}
	if game.Manifest != nil && game.Manifest.Version > 0 {
		return game.Manifest.Version
	}
	if game.Status == model.GameReady {
		return 1
	}
	return 0
}

func previewArtifactKey(version int, asset string) string {
	return fmt.Sprintf("preview/v%d/%s", version, asset)
}

func previewAssetURL(gameID string, version int, asset string) string {
	return "/v1/games/" + url.PathEscape(gameID) + "/previews/" +
		strconv.Itoa(version) + "/" + asset
}

func (s *Server) ownedPublishedGame(w http.ResponseWriter, r *http.Request) (*model.Game, int, bool) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return nil, 0, false
	}
	if userID(r) == "" || userID(r) != game.OwnerID {
		writeErr(w, http.StatusForbidden, "only the creator can manage previews")
		return nil, 0, false
	}
	if game.Status != model.GameReady || game.Quarantined || publishedGameVersion(game) < 1 {
		writeErr(w, http.StatusConflict, "preview assets require a ready published game")
		return nil, 0, false
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version != publishedGameVersion(game) {
		writeErr(w, http.StatusConflict, "preview version does not match the published game version")
		return nil, 0, false
	}
	return game, version, true
}

func (s *Server) handleUploadPreviewAsset(w http.ResponseWriter, r *http.Request) {
	game, version, ok := s.ownedPublishedGame(w, r)
	if !ok {
		return
	}
	asset := r.PathValue("asset")
	var limit int64
	var allowedTypes []string
	switch asset {
	case "poster.png", "poster.jpg":
		limit = maxPreviewPosterBytes
		allowedTypes = []string{"image/png", "image/jpeg"}
	case "clip.mp4":
		limit = maxPreviewClipBytes
		allowedTypes = []string{"video/mp4"}
	default:
		writeErr(w, http.StatusBadRequest, "asset must be poster.png, poster.jpg, or clip.mp4")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeErr(w, http.StatusUnsupportedMediaType, "a supported Content-Type is required")
		return
	}
	allowed := false
	for _, candidate := range allowedTypes {
		if contentType == candidate {
			allowed = true
			break
		}
	}
	if !allowed || (strings.HasSuffix(asset, ".png") && contentType != "image/png") ||
		(strings.HasSuffix(asset, ".jpg") && contentType != "image/jpeg") {
		writeErr(w, http.StatusUnsupportedMediaType, "preview asset type does not match its filename")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit+1))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "preview asset exceeds its size limit")
		return
	}
	if int64(len(data)) == 0 || int64(len(data)) > limit {
		writeErr(w, http.StatusRequestEntityTooLarge, "preview asset is empty or exceeds its size limit")
		return
	}
	if !validPreviewBytes(asset, data) {
		writeErr(w, http.StatusUnsupportedMediaType, "preview asset bytes do not match the declared format")
		return
	}
	key := previewArtifactKey(version, asset)
	if existing, _, found := s.store.GetArtifact(game.ID, key); found {
		if !bytes.Equal(existing, data) {
			writeErr(w, http.StatusConflict, "versioned preview assets are immutable")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.store.PutArtifact(game.ID, key, data, contentType)
	w.WriteHeader(http.StatusCreated)
}

func validPreviewBytes(asset string, data []byte) bool {
	switch asset {
	case "poster.png", "poster.jpg":
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 4096 || config.Height > 4096 {
			return false
		}
		if asset == "poster.png" {
			return format == "png"
		}
		return format == "jpeg"
	case "clip.mp4":
		return len(data) >= 12 && string(data[4:8]) == "ftyp"
	default:
		return false
	}
}

func validPreviewColor(color string) bool {
	if color == "" {
		return true
	}
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	_, err := hex.DecodeString(color[1:])
	return err == nil
}

func (s *Server) handleSetPreviewMetadata(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if userID(r) == "" || userID(r) != game.OwnerID {
		writeErr(w, http.StatusForbidden, "only the creator can manage previews")
		return
	}
	version := publishedGameVersion(game)
	if game.Status != model.GameReady || game.Quarantined || version < 1 {
		writeErr(w, http.StatusConflict, "preview metadata requires a ready published game")
		return
	}
	var req previewMetadataRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Version != version {
		writeErr(w, http.StatusConflict, "preview version does not match the published game version")
		return
	}
	if req.Status != "pending" && req.Status != "ready" && req.Status != "failed" {
		writeErr(w, http.StatusBadRequest, "status must be pending, ready, or failed")
		return
	}
	if req.Aspect != 0 && (req.Aspect < 0.25 || req.Aspect > 4) {
		writeErr(w, http.StatusBadRequest, "aspect must be between 0.25 and 4")
		return
	}
	if req.DurationMS < 0 || req.DurationMS > 10000 {
		writeErr(w, http.StatusBadRequest, "duration_ms must be between 0 and 10000")
		return
	}
	if !validPreviewColor(req.DominantColor) {
		writeErr(w, http.StatusBadRequest, "dominant_color must be a hex color such as #AABBCC")
		return
	}
	posterAsset := "poster.png"
	if _, _, found := s.store.GetArtifact(game.ID, previewArtifactKey(version, posterAsset)); !found {
		posterAsset = "poster.jpg"
	}
	_, _, hasPoster := s.store.GetArtifact(game.ID, previewArtifactKey(version, posterAsset))
	_, _, hasClip := s.store.GetArtifact(game.ID, previewArtifactKey(version, "clip.mp4"))
	if (req.Status == "ready" && (!hasPoster || !hasClip || req.DurationMS == 0)) ||
		(req.Status == "failed" && !hasPoster) {
		writeErr(w, http.StatusConflict, "ready previews require a poster and clip; failed previews require a poster fallback")
		return
	}
	metadata := &model.PreviewMetadata{
		Version: version, Status: req.Status, Aspect: req.Aspect,
		DurationMS: req.DurationMS, DominantColor: strings.ToUpper(req.DominantColor),
	}
	if hasPoster && req.Status != "pending" {
		metadata.PosterURL = previewAssetURL(game.ID, version, posterAsset)
	}
	if hasClip && req.Status == "ready" {
		metadata.ClipURL = previewAssetURL(game.ID, version, "clip.mp4")
	}
	game.Preview = metadata
	s.store.UpdateGame(game)
	writeJSON(w, http.StatusOK, metadata)
}

func (s *Server) handleGetPreviewAsset(w http.ResponseWriter, r *http.Request) {
	game, ok := s.deliverableGame(w, r)
	if !ok {
		return
	}
	version, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || version != publishedGameVersion(game) || game.Preview == nil ||
		game.Preview.Version != version || (game.Preview.Status != "ready" && game.Preview.Status != "failed") {
		w.Header().Set("Cache-Control", "private, no-store")
		writeErr(w, http.StatusNotFound, "preview asset not found")
		return
	}
	asset := r.PathValue("asset")
	expectedURL := ""
	contentType := ""
	switch asset {
	case "poster.png", "poster.jpg":
		expectedURL = game.Preview.PosterURL
		if asset == "poster.png" {
			contentType = "image/png"
		} else {
			contentType = "image/jpeg"
		}
	case "clip.mp4":
		expectedURL = game.Preview.ClipURL
		contentType = "video/mp4"
	default:
		writeErr(w, http.StatusNotFound, "preview asset not found")
		return
	}
	if expectedURL == "" || expectedURL != previewAssetURL(game.ID, version, asset) {
		writeErr(w, http.StatusNotFound, "preview asset not found")
		return
	}
	data, storedType, found := s.store.GetArtifact(game.ID, previewArtifactKey(version, asset))
	if !found {
		writeErr(w, http.StatusNotFound, "preview asset content not found")
		return
	}
	if storedType != "" {
		contentType = storedType
	}
	etag := strongETag(data)
	w.Header().Set("ETag", etag)
	if packageIsPublicImmutable(game) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Vary", "Accept-Encoding")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Vary", "X-User-ID")
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == etag || r.Header.Get("If-None-Match") == "*" {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
