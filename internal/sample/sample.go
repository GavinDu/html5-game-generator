// Package sample seeds the built-in sample game: "Bobo's Rainbow", generated
// from a kid's crayon drawing of a crowned bear, hearts, and a rainbow.
// The bear and kitten sprites are crops of the actual drawing (style-
// preserving by construction); hearts and rainbow bands are procedural
// stand-ins from internal/media. Seeding is idempotent.
package sample

import (
	"doodlequest/internal/media"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
	_ "embed"
	"encoding/base64"
	"image/color"
	"time"
)

//go:embed assets/game.js
var gameJS string

// NOTE: the bear/kitten PNGs are committed as base64 text because the
// GitHub file API used for pushes corrupts binary uploads (verified
// 2026-09-30 via blob-SHA mismatch). To restore real PNGs: web-upload
// assets/bear.png and assets/kitten.png through github.com and switch
// these embeds back to the .png files.
//go:embed assets/bear.png.b64
var bearB64 string

//go:embed assets/kitten.png.b64
var kittenB64 string

// GameID is the stable id of the sample game, so players can link it.
const GameID = "sample-rainbow-bear"

var pink = color.NRGBA{236, 112, 168, 255}
var blue = color.NRGBA{93, 173, 226, 255}

// Seed publishes the sample game into the store if it isn't there yet.
func Seed(st store.Store) {
	if _, ok := st.GetGame(GameID); ok {
		return
	}
	now := time.Now()
	elements := []model.Element{
		{ID: "el_1", Label: "bear prince", Kind: "character", Description: "Bobo, a brown furry bear with blue eyes wearing a tiny golden heart-crown (cropped from the kid's drawing)"},
		{ID: "el_2", Label: "hearts", Kind: "prop", Description: "pink and blue hearts floating in the sky"},
		{ID: "el_3", Label: "rainbow", Kind: "background", Description: "a six-band rainbow arching over the meadow"},
		{ID: "el_4", Label: "kitten", Kind: "character", Description: "a small kitten sketched in pencil under the rainbow (cropped from the kid's drawing)"},
	}
	game := &model.Game{
		ID:         GameID,
		Title:      "Bobo's Rainbow",
		OwnerID:    "doodlequest",
		Elements:   elements,
		Intent:     "Catch falling hearts to repaint the fading rainbow so the kitten can cross",
		Visibility: model.VisibilityPublic,
		Status:     model.GameReady,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m := &model.BundleManifest{
		GameID:   GameID,
		Title:    "Bobo's Rainbow",
		Version:  1,
		Shim:     "gameshim/v1",
		Entry:    "game.js",
		Controls: []string{"tap"},
	}
	m.Assets.Sprites = []model.SpriteAsset{
		{ID: "bear", Src: "sprites/bear.png", Source: "kid-drawing", Label: "bear prince"},
		{ID: "kitten", Src: "sprites/kitten.png", Source: "kid-drawing", Label: "kitten"},
		{ID: "heart", Src: "sprites/heart.png", Source: "procedural", Label: "pink heart"},
		{ID: "heart_blue", Src: "sprites/heart_blue.png", Source: "procedural", Label: "blue heart"},
	}
	for i := 0; i < 6; i++ {
		m.Assets.Sprites = append(m.Assets.Sprites, model.SpriteAsset{
			ID:     "band" + string(rune('0'+i)),
			Src:    "sprites/band" + string(rune('0'+i)) + ".png",
			Source: "procedural",
			Label:  "rainbow band",
		})
	}
	m.Assets.Music = model.MusicAsset{Src: "audio/theme.wav", Mood: "calm", Source: "library"}
	m.Visibility = model.VisibilityPublic
	game.Manifest = m
	st.CreateGame(game)

	st.PutArtifact(GameID, "game.js", []byte(gameJS), "application/javascript")
	bearPNG, err := base64.StdEncoding.DecodeString(bearB64)
	if err != nil {
		panic("sample: corrupt embedded bear sprite: " + err.Error())
	}
	kittenPNG, err := base64.StdEncoding.DecodeString(kittenB64)
	if err != nil {
		panic("sample: corrupt embedded kitten sprite: " + err.Error())
	}
	st.PutArtifact(GameID, "sprites/bear.png", bearPNG, "image/png")
	st.PutArtifact(GameID, "sprites/kitten.png", kittenPNG, "image/png")
	st.PutArtifact(GameID, "sprites/heart.png", media.HeartPNG(pink), "image/png")
	st.PutArtifact(GameID, "sprites/heart_blue.png", media.HeartPNG(blue), "image/png")
	for i := 0; i < 6; i++ {
		name := "sprites/band" + string(rune('0'+i)) + ".png"
		st.PutArtifact(GameID, name, media.RainbowBandPNG(i), "image/png")
	}
	st.PutArtifact(GameID, "audio/theme.wav", media.MusicWAV("calm"), "audio/wav")
}
