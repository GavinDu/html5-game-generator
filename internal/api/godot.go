package api

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Godot web versions of the games.
//
// The repo stays text-only (the GitHub connector corrupts binary uploads),
// so compiled web exports are never committed. godot/web-exports.json maps
// each game to a zip of its Godot web export; the server downloads and
// extracts it on first request into godot/<game>/web/ (gitignored) and
// serves it with the COOP/COEP headers Godot 4 web builds require.

type godotExport struct {
	Title string `json:"title"`
	Zip   string `json:"zip"`
}

var (
	godotManifest   map[string]godotExport
	godotManifestMu sync.Mutex
	godotProvMu     sync.Mutex
)

var errGodotUnavailable = errors.New("godot web build not available")

func loadGodotManifest() map[string]godotExport {
	godotManifestMu.Lock()
	defer godotManifestMu.Unlock()
	if godotManifest != nil {
		return godotManifest
	}
	godotManifest = map[string]godotExport{}
	b, err := os.ReadFile(filepath.Join("godot", "web-exports.json"))
	if err != nil {
		return godotManifest
	}
	_ = json.Unmarshal(b, &godotManifest)
	return godotManifest
}

// ensureGodotWeb provisions godot/<game>/web/ on first use.
func ensureGodotWeb(game string) error {
	godotProvMu.Lock()
	defer godotProvMu.Unlock()
	dir := filepath.Join("godot", game, "web")
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
		return nil
	}
	m := loadGodotManifest()
	exp, ok := m[game]
	if !ok || exp.Zip == "" {
		return errGodotUnavailable
	}
	tmp, err := os.CreateTemp("", "godot-*.zip")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	resp, err := http.Get(exp.Zip) //nolint:gosec // URL comes from our own committed manifest
	if err != nil {
		_ = tmp.Close()
		return err
	}
	if resp.StatusCode != http.StatusOK {
		_ = tmp.Close()
		_ = resp.Body.Close()
		return errGodotUnavailable
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = tmp.Close()
		_ = resp.Body.Close()
		return err
	}
	_ = tmp.Close()
	_ = resp.Body.Close()

	zr, err := zip.OpenReader(tmpName)
	if err != nil {
		return err
	}
	defer zr.Close()
	stage := dir + ".staging"
	_ = os.RemoveAll(stage)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Clean(f.Name)
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			continue
		}
		dst := filepath.Join(stage, name)
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = rc.Close()
		_ = out.Close()
		if err != nil {
			return err
		}
	}
	if err := os.Rename(stage, dir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return errGodotUnavailable
	}
	return nil
}

// GET /v1/godot — list the Godot web versions.
func (s *Server) handleGodotList(w http.ResponseWriter, r *http.Request) {
	m := loadGodotManifest()
	type item struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Ready bool   `json:"ready"`
		URL   string `json:"url"`
	}
	out := []item{}
	for id, e := range m {
		_, err := os.Stat(filepath.Join("godot", id, "web", "index.html"))
		out = append(out, item{ID: id, Title: e.Title, Ready: err == nil, URL: "/godot/" + id + "/"})
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /godot/{game}/... — serve a Godot web export with the
// cross-origin isolation headers its engine requires.
func (s *Server) handleGodot(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/godot/")
	game, sub := rest, "index.html"
	if i := strings.Index(rest, "/"); i >= 0 {
		game, sub = rest[:i], rest[i+1:]
		if sub == "" {
			sub = "index.html"
		}
	}
	m := loadGodotManifest()
	if _, ok := m[game]; !ok {
		http.NotFound(w, r)
		return
	}
	if err := ensureGodotWeb(game); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "godot version is still downloading, try again shortly")
		return
	}
	clean := filepath.Clean(filepath.FromSlash(sub))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	if strings.HasSuffix(clean, ".wasm") {
		w.Header().Set("Content-Type", "application/wasm")
	}
	http.ServeFile(w, r, filepath.Join("godot", game, "web", clean))
}
