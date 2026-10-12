package api

import (
	"net/http"
	"path"
	"strings"

	"doodlequest/internal/model"
)

// PackageVersion is the data-package descriptor version.
const PackageVersion = "1.0"

// RuntimeRequirement is the player-runtime handshake, parsed from the
// manifest's shim field (e.g. "gameshim/v1"). A bundled-runtime client
// checks Required against its embedded runtime before loading game data;
// an empty Required means "runtime unknown: use the legacy remote path".
type RuntimeRequirement struct {
	Required string `json:"required"`
	Kind     string `json:"kind"`
	Version  string `json:"version"`
}

// PackageAsset is one downloadable datum in the package. URLs are relative
// to the backend base URL the client already talks to.
type PackageAsset struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "entry" | "sprite" | "music"
	URL    string `json:"url"`
	Mime   string `json:"mime,omitempty"`
	Source string `json:"source,omitempty"` // kid-drawing | procedural | library | generated
	Label  string `json:"label,omitempty"`
}

// PackageData is the data-only payload: everything a bundled-runtime client
// fetches. It carries no execution semantics; the client's embedded runtime
// interprets it.
type PackageData struct {
	Entry   PackageAsset  `json:"entry"`
	Sprites []PackageAsset `json:"sprites"`
	Music   *PackageAsset  `json:"music,omitempty"`
}

// GamePackage is the data-plane descriptor for one game.
type GamePackage struct {
	PackageVersion string              `json:"packageVersion"`
	GameID         string              `json:"gameId"`
	Runtime        RuntimeRequirement  `json:"runtime"`
	Manifest       *model.BundleManifest `json:"manifest"`
	Data           PackageData         `json:"data"`
}

// parseRuntime splits a manifest shim field ("gameshim/v1") into the
// handshake triple.
func parseRuntime(shim string) RuntimeRequirement {
	req := RuntimeRequirement{Required: shim}
	if i := strings.Index(shim, "/"); i > 0 && i < len(shim)-1 {
		req.Kind = shim[:i]
		req.Version = shim[i+1:]
	}
	return req
}

func mimeFor(src string) string {
	switch strings.ToLower(path.Ext(src)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	}
	return ""
}

// GET /v1/games/{id}/package — the data-plane descriptor for one game.
//
// For bundled-runtime clients (e.g. the iOS player container): the player
// runtime ships inside the client, so the client fetches only this
// descriptor plus the referenced data URLs. The existing /bundle routes are
// untouched: the browser player keeps using them as before.
func (s *Server) handleGetPackage(w http.ResponseWriter, r *http.Request) {
	game, ok := s.store.GetGame(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "game not found")
		return
	}
	if game.Manifest == nil {
		writeErr(w, http.StatusConflict, "package not published yet")
		return
	}
	m := game.Manifest
	base := "/v1/games/" + game.ID + "/bundle"
	pkg := GamePackage{
		PackageVersion: PackageVersion,
		GameID:         game.ID,
		Runtime:        parseRuntime(m.Shim),
		Manifest:       m,
		Data: PackageData{
			Entry: PackageAsset{
				ID: "entry", Kind: "entry",
				URL: base + "/" + m.Entry, Mime: mimeFor(m.Entry),
				Label: "game data interpreted by the bundled runtime",
			},
		},
	}
	for _, sp := range m.Assets.Sprites {
		pkg.Data.Sprites = append(pkg.Data.Sprites, PackageAsset{
			ID: sp.ID, Kind: "sprite",
			URL: base + "/" + sp.Src, Mime: mimeFor(sp.Src),
			Source: sp.Source, Label: sp.Label,
		})
	}
	if m.Assets.Music.Src != "" {
		pkg.Data.Music = &PackageAsset{
			ID: "music", Kind: "music",
			URL: base + "/" + m.Assets.Music.Src, Mime: mimeFor(m.Assets.Music.Src),
			Source: m.Assets.Music.Source, Label: m.Assets.Music.Mood,
		}
	}
	writeJSON(w, http.StatusOK, pkg)
}
