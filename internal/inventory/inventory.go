// Package inventory loads the shipped game inventory: one folder per game
// under inventory/<6-char-id>/, each holding a manifest.json, game.js, and
// an assets/ directory. Sprites are stored as base64 text (binary-safe for
// git); they are decoded at server start, never committed as raw images.
//
// Loading is idempotent: a game whose id is already in the store is skipped.
package inventory

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"doodlequest/internal/gameir"
	"doodlequest/internal/media"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

// dirName is the required shape of a game folder: a unique 6-char string.
var dirName = regexp.MustCompile(`^[a-z0-9]{6}$`)

type spriteSpec struct {
	ID         string `json:"id"`
	Src        string `json:"src"`
	Mime       string `json:"mime"`
	Source     string `json:"source"`
	Label      string `json:"label"`
	File       string `json:"file,omitempty"`       // .b64 text path, relative to the game folder
	Procedural string `json:"procedural,omitempty"` // "heart:#RRGGBB" | "band:N"
}

type musicSpec struct {
	Src    string `json:"src"`
	Mood   string `json:"mood"`
	Source string `json:"source"`
}

// manifest is the on-disk description of one inventory game.
type manifest struct {
	GameID     string          `json:"game_id"`
	Title      string          `json:"title"`
	Owner      string          `json:"owner"`
	Intent     string          `json:"intent"`
	Visibility string          `json:"visibility"`
	Version    int             `json:"version"`
	Shim       string          `json:"shim"`
	Entry      string          `json:"entry"`
	Controls   []string        `json:"controls"`
	Elements   []model.Element `json:"elements"`
	Music      musicSpec       `json:"music"`
	Sprites    []spriteSpec    `json:"sprites"`
}

// Load reads every inventory/<6-char-id>/ folder under dir and seeds the
// games into the store.
func Load(st store.Store, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("inventory: cannot read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && dirName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return fmt.Errorf("inventory: no game folders (<6-char-id>) in %s", dir)
	}
	seen := map[string]string{}
	for _, n := range names {
		gdir := filepath.Join(dir, n)
		m, err := readManifest(gdir)
		if err != nil {
			return err
		}
		if prev, dup := seen[m.GameID]; dup {
			return fmt.Errorf("inventory: duplicate game_id %q in %s and %s", m.GameID, prev, n)
		}
		seen[m.GameID] = n
		if err := seedGame(st, gdir, m); err != nil {
			return err
		}
	}
	return nil
}

func readManifest(gdir string) (*manifest, error) {
	raw, err := os.ReadFile(filepath.Join(gdir, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("inventory: %s: cannot read manifest.json: %w", gdir, err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("inventory: %s: bad manifest.json: %w", gdir, err)
	}
	if m.GameID == "" || m.Title == "" || m.Entry == "" {
		return nil, fmt.Errorf("inventory: %s: manifest needs game_id, title, entry", gdir)
	}
	if len(m.Sprites) == 0 {
		return nil, fmt.Errorf("inventory: %s: manifest lists no sprites", gdir)
	}
	return &m, nil
}

func seedGame(st store.Store, gdir string, m *manifest) error {
	if _, ok := st.GetGame(m.GameID); ok {
		return nil // already seeded
	}
	code, err := os.ReadFile(filepath.Join(gdir, m.Entry))
	if err != nil {
		return fmt.Errorf("inventory: %s: cannot read %s: %w", gdir, m.Entry, err)
	}
	now := time.Now()
	game := &model.Game{
		ID:         m.GameID,
		Title:      m.Title,
		OwnerID:    m.Owner,
		Elements:   m.Elements,
		Intent:     m.Intent,
		Visibility: model.Visibility(m.Visibility),
		Status:     model.GameReady,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	bm := &model.BundleManifest{
		GameID:   m.GameID,
		Title:    m.Title,
		Version:  m.Version,
		Shim:     m.Shim,
		Entry:    m.Entry,
		Controls: m.Controls,
	}
	for _, s := range m.Sprites {
		bm.Assets.Sprites = append(bm.Assets.Sprites, model.SpriteAsset{
			ID: s.ID, Src: s.Src, Source: s.Source, Label: s.Label,
		})
	}
	bm.Assets.Music = model.MusicAsset{Src: m.Music.Src, Mood: m.Music.Mood, Source: m.Music.Source}
	bm.Visibility = model.Visibility(m.Visibility)
	game.Manifest = bm
	// A game folder may carry the canonical Game IR document
	// (game-ir.json), as written by the generation pipeline. When present
	// it must decode, validate, and belong to this game — a broken IR
	// fails the load instead of silently 409ing at request time. When
	// absent the game simply has no published IR yet.
	if raw, err := os.ReadFile(filepath.Join(gdir, "game-ir.json")); err == nil {
		doc, err := gameir.DecodeAndValidate(raw)
		if err != nil {
			return fmt.Errorf("inventory: %s: game-ir.json: %w", gdir, err)
		}
		if doc.Game.ID != m.GameID {
			return fmt.Errorf("inventory: %s: game-ir.json game id %q != manifest game_id %q", gdir, doc.Game.ID, m.GameID)
		}
		game.GameIR = doc
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inventory: %s: cannot read game-ir.json: %w", gdir, err)
	}
	st.CreateGame(game)

	st.PutArtifact(m.GameID, m.Entry, code, "application/javascript")
	for _, s := range m.Sprites {
		img, err := spriteBytes(gdir, s)
		if err != nil {
			return fmt.Errorf("inventory: %s: sprite %q: %w", gdir, s.ID, err)
		}
		st.PutArtifact(m.GameID, s.Src, img, s.Mime)
	}
	st.PutArtifact(m.GameID, m.Music.Src, media.MusicWAV(m.Music.Mood), "audio/wav")
	return nil
}

func spriteBytes(gdir string, s spriteSpec) ([]byte, error) {
	switch {
	case s.File != "":
		raw, err := os.ReadFile(filepath.Join(gdir, s.File))
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", s.File, err)
		}
		dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, fmt.Errorf("corrupt base64 in %s: %w", s.File, err)
		}
		return dec, nil
	case s.Procedural != "":
		return proceduralPNG(s.Procedural)
	default:
		return nil, fmt.Errorf("sprite needs file or procedural")
	}
}

// proceduralPNG renders the media-package stand-ins referenced by manifests.
func proceduralPNG(spec string) ([]byte, error) {
	if strings.HasPrefix(spec, "heart:") {
		c, err := parseHexColor(strings.TrimPrefix(spec, "heart:"))
		if err != nil {
			return nil, err
		}
		return media.HeartPNG(c), nil
	}
	if strings.HasPrefix(spec, "band:") {
		n, err := strconv.Atoi(strings.TrimPrefix(spec, "band:"))
		if err != nil || n < 0 || n > 5 {
			return nil, fmt.Errorf("bad band spec %q (want band:0..5)", spec)
		}
		return media.RainbowBandPNG(n), nil
	}
	return nil, fmt.Errorf("unknown procedural sprite %q", spec)
}

func parseHexColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("bad color %q (want #RRGGBB)", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("bad color %q", s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}
