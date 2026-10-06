# 飘带补天 Ribbon Mends the Sky — Godot rework

Godot 4.7 rework of the GameShim original (`inventory/u46ex5/`).
Same rules — stitch 6 sky-holes per round, endless 1.25x 关 meta-levels,
bonk demons, 3 lives — rebuilt with Godot-native features:

- **Physics areas** for stitch/collect/demon-hit detection
- **Thread-flash Line2D** when a hole is stitched, stitch particle bursts
- **Juice**: demon squash-and-tilt while falling, hero invulnerability blink,
  camera shake on hit, twinkling starfield, drifting clouds, floating popups
- **Procedural audio** (`synth.gd`): playful pentatonic music-box loop +
  synthesized SFX, zero binary assets
- Kid's art (hero, holes, demons, shards, bg) reused from committed `.b64` text

## Run

```sh
python3 ../tools/decode_assets.py
godot --headless --path . -- --self-test   # expect PASS
```

`--api=http://host:port` enables the game-over leaderboard POST.
