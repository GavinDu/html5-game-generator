// Package sample seeds the second built-in sample game: "飘带补天 Ribbon
// Mends the Sky", generated from a photo of a little girl in a fairy dress
// riding a swirl cloud, holding a golden staff, ribbons flowing.
// The hero sprite is a cutout of the actual photo (style-preserving by
// construction); the sky, demon, holes, shards and sparkles are generated.
// Seeding is idempotent.
package sample

import (
	_ "embed"
	"encoding/base64"
	"time"

	"doodlequest/internal/media"
	"doodlequest/internal/model"
	"doodlequest/internal/store"
)

//go:embed assets-sky/game.js
var skyGameJS string

// NOTE: PNGs are committed as base64 text because the GitHub file API used
// for pushes corrupts binary uploads (verified 2026-09-30 via blob-SHA
// mismatch). To restore real PNGs: web-upload the decoded files through
// github.com and switch these embeds back to the image files.
//go:embed assets-sky/bg.jpg.b64
var skyBgB64 string

//go:embed assets-sky/hero.png.b64
var skyHeroB64 string

//go:embed assets-sky/demon.png.b64
var skyDemonB64 string

//go:embed assets-sky/hole.png.b64
var skyHoleB64 string

//go:embed assets-sky/hole_closed.png.b64
var skyHoleClosedB64 string

//go:embed assets-sky/shard.png.b64
var skyShardB64 string

//go:embed assets-sky/spark.png.b64
var skySparkB64 string

// SkyGameID is the stable id of the sample game, so players can link it.
const SkyGameID = "sample-ribbon-sky"

func mustDecodeB64(name, s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic("sample: corrupt embedded sky asset " + name + ": " + err.Error())
	}
	return b
}

// SeedSky publishes "飘带补天 Ribbon Mends the Sky" into the store if it
// isn't there yet.
func SeedSky(st store.Store) {
	if _, ok := st.GetGame(SkyGameID); ok {
		return
	}
	now := time.Now()
	elements := []model.Element{
		{ID: "el_1", Label: "little girl hero", Kind: "character", Description: "the little girl in the white-blue fairy dress, riding her cloud (cut out from the photo)"},
		{ID: "el_2", Label: "golden staff", Kind: "prop", Description: "her golden carved staff, for bonking wind demons (in the hero cutout)"},
		{ID: "el_3", Label: "swirl cloud", Kind: "vehicle", Description: "the somersault cloud she rides (in the hero cutout)"},
		{ID: "el_4", Label: "flowing ribbons", Kind: "prop", Description: "blue-white ribbons streaming behind her — the mending magic (in the hero cutout)"},
		{ID: "el_5", Label: "sky-hole", Kind: "hazard", Description: "a dark tear in the sky the wind demons ripped open"},
		{ID: "el_6", Label: "wind demon", Kind: "character", Description: "a grumpy little storm blob; tap to bonk it with the staff"},
		{ID: "el_7", Label: "rainbow shard", Kind: "collectible", Description: "a shard of the nine-colored deer's rainbow; catch for bonus"},
	}
	game := &model.Game{
		ID:         SkyGameID,
		Title:      "飘带补天 Ribbon Mends the Sky",
		OwnerID:    "doodlequest",
		Elements:   elements,
		Intent:     "Fly into the dark sky-holes so the ribbons stitch them shut; bonk wind demons with the golden staff",
		Visibility: model.VisibilityPublic,
		Status:     model.GameReady,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m := &model.BundleManifest{
		GameID:   SkyGameID,
		Title:    "飘带补天 Ribbon Mends the Sky",
		Version:  1,
		Shim:     "gameshim/v1",
		Entry:    "game.js",
		Controls: []string{"tap"},
	}
	m.Assets.Sprites = []model.SpriteAsset{
		{ID: "bg", Src: "sprites/bg.jpg", Source: "generated", Label: "sunset cloud sea"},
		{ID: "hero", Src: "sprites/hero.png", Source: "photo", Label: "little girl hero on cloud"},
		{ID: "demon", Src: "sprites/demon.png", Source: "generated", Label: "wind demon"},
		{ID: "hole", Src: "sprites/hole.png", Source: "generated", Label: "open sky-hole"},
		{ID: "hole_closed", Src: "sprites/hole_closed.png", Source: "generated", Label: "stitched sky-hole"},
		{ID: "shard", Src: "sprites/shard.png", Source: "generated", Label: "rainbow shard"},
		{ID: "spark", Src: "sprites/spark.png", Source: "generated", Label: "sparkle burst"},
	}
	m.Assets.Music = model.MusicAsset{Src: "audio/theme.wav", Mood: "playful", Source: "synthesized"}
	m.Visibility = model.VisibilityPublic
	game.Manifest = m
	st.CreateGame(game)

	st.PutArtifact(SkyGameID, "game.js", []byte(skyGameJS), "application/javascript")
	st.PutArtifact(SkyGameID, "sprites/bg.jpg", mustDecodeB64("bg.jpg", skyBgB64), "image/jpeg")
	st.PutArtifact(SkyGameID, "sprites/hero.png", mustDecodeB64("hero.png", skyHeroB64), "image/png")
	st.PutArtifact(SkyGameID, "sprites/demon.png", mustDecodeB64("demon.png", skyDemonB64), "image/png")
	st.PutArtifact(SkyGameID, "sprites/hole.png", mustDecodeB64("hole.png", skyHoleB64), "image/png")
	st.PutArtifact(SkyGameID, "sprites/hole_closed.png", mustDecodeB64("hole_closed.png", skyHoleClosedB64), "image/png")
	st.PutArtifact(SkyGameID, "sprites/shard.png", mustDecodeB64("shard.png", skyShardB64), "image/png")
	st.PutArtifact(SkyGameID, "sprites/spark.png", mustDecodeB64("spark.png", skySparkB64), "image/png")
	st.PutArtifact(SkyGameID, "audio/theme.wav", media.MusicWAV("playful"), "audio/wav")
}
