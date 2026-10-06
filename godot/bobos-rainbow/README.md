# Bobo's Rainbow — Godot rework

Godot 4.7 rework of the GameShim original (`inventory/13ui1k/`).
Same rules — catch 12 hearts to clear the round, endless 1.25x meta-levels,
3 misses end the run — rebuilt with Godot-native features:

- **Physics areas** for catch detection (`Area2D.area_entered`, no manual AABB)
- **Procedural heart textures** drawn in code (`heart_tex.gd`); the **rainbow
  arch drawn natively** with `draw_arc`, bands pop in and shimmer (`rainbow.gd`)
- **Juice**: catch particle bursts, floating score popups, Bobo squash &
  stretch, combo pitch-rising SFX, camera shake on miss, drifting clouds
- **Procedural audio** (`synth.gd`): music-box loop + synthesized SFX, zero
  binary assets
- Kid's art (bear, kitten) reused from the committed `.b64` text

## Run

```sh
python3 ../tools/decode_assets.py   # materialize assets/*.png from .b64 text
godot --path .                      # open in the editor, or:
godot --headless --path . -- --self-test   # headless bot plays; expect PASS
```

`--api=http://host:port` enables the game-over leaderboard POST.

## Web export

Project uses `gl_compatibility` + `CPUParticles2D` only, so it exports cleanly
to Web (WASM) for the WKWebView player path, as well as iOS/Android/desktop.
