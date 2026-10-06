# Escape from the Injection — Godot rework

Godot 4.7 rework of the GameShim original (`inventory/2uowg9/`).
Same rules — 3-lane tap dodge, the chaser-gap is your health, new place
every 40s at 1.12x, finally caught with tears — rebuilt with Godot-native
features:

- **Parallax scrolling** escape route (the web original was static): drifting
  dots + rushing lane dashes at different depths, backdrops crossfade per level
- **Tweened lane changes** with squash & stretch and lean; **the chaser
  visibly looms larger** as the gap shrinks; **speed lines** scale with pace
- **Physics areas** for syringe hits and star pickups; **camera shake** +
  red vignette flash on hit; floating score popups
- **Procedural audio** (`synth.gd`): adventurous music-box loop + synthesized
  SFX, zero binary assets
- Kid's art (blob, chaser, syringe, star, backdrops) reused from `.b64` text

## Run

```sh
python3 ../tools/decode_assets.py
godot --headless --path . -- --self-test   # expect PASS (dodge to LV 2, then caught)
```

`--api=http://host:port` enables the game-over leaderboard POST.
