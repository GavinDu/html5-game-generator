package api

import (
	"net/http"
	"net/url"

	"doodlequest/internal/gameir"
)

const PackageVersion = "2.0"

type GamePackageAsset struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	URI      string `json:"uri"`
	URL      string `json:"url"`
	MIMEType string `json:"mimeType,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Source   string `json:"source,omitempty"`
	AltText  string `json:"altText,omitempty"`
}

// GamePackage is the canonical, engine-independent delivery envelope.
// The IR keeps package-relative asset URIs; URLs here are delivery locations.
type GamePackage struct {
	PackageVersion string             `json:"packageVersion"`
	GameID         string             `json:"gameId"`
	IR             *gameir.Document   `json:"ir"`
	Assets         []GamePackageAsset `json:"assets"`
}

func (s *Server) handleGetPackage(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "Game IR has not been published")
		return
	}
	pkg := GamePackage{
		PackageVersion: PackageVersion,
		GameID: game.ID,
		IR: game.GameIR,
		Assets: make([]GamePackageAsset, 0, len(game.GameIR.Assets)),
	}
	for _, asset := range game.GameIR.Assets {
		pkg.Assets = append(pkg.Assets, GamePackageAsset{
			ID: asset.ID, Kind: asset.Kind, URI: asset.URI,
			URL: "/v1/games/" + game.ID + "/assets/" + url.PathEscape(asset.ID),
			MIMEType: asset.MIMEType, SHA256: asset.SHA256,
			Source: asset.Source, AltText: asset.AltText,
		})
	}
	writeJSON(w, http.StatusOK, pkg)
}

func (s *Server) handleGetGameAsset(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.GameIR == nil {
		writeErr(w, http.StatusConflict, "Game IR has not been published")
		return
	}
	assetID := r.PathValue("asset")
	for _, asset := range game.GameIR.Assets {
		if asset.ID != assetID {
			continue
		}
		data, contentType, found := s.store.GetArtifact(game.ID, asset.URI)
		if !found {
			writeErr(w, http.StatusNotFound, "asset content not found")
			return
		}
		if contentType == "" {
			contentType = asset.MIMEType
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	writeErr(w, http.StatusNotFound, "asset not found in Game IR")
}
