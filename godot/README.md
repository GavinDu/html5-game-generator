# Godot reworks

Godot 4.7 reworks of the three GameShim web games, rebuilt with Godot-native
features: physics `Area2D` collision, `CPUParticles2D`, tweens (squash &
stretch), `Camera2D` shake, parallax, and fully procedural audio (`synth.gd`
music-box loops + synthesized SFX — zero binary audio).

| Project | Original | Highlights |
|---|---|---|
| `bobos-rainbow/` | `inventory/13ui1k/` | Hearts + rainbow arch drawn natively in code; catch bursts, floating popups, combo pitch-rise |
| `ribbon-sky/` | `inventory/u46ex5/` | Thread-flash `Line2D` stitches, demon squash-and-tilt, twinkling starfield |
| `escape-injection/` | `inventory/2uowg9/` | Parallax scrolling route, tweened lane changes, chaser looms larger as it closes in, speed lines |

The web originals keep working untouched; these are the "best features"
editions. Projects are platform-agnostic — they export to Web (WASM, keeps
the WKWebView/iPhone path), iOS, Android, and desktop. Only
`gl_compatibility` + CPU particles are used, so Web export is clean.

## Setup

The repo never stores raw binary (GitHub connector file tools corrupt it),
so game art lives as `.b64` text under `inventory/<id>/assets/`. Materialize
it before opening a project:

```sh
python3 godot/tools/decode_assets.py
```

(`godot/*/assets/*.png|jpg` are gitignored; the script regenerates them.
Hearts and rainbow bands are drawn procedurally in-engine — no binary needed.)

## Play them in the DoodleQuest player

Each project is exported to Web (WASM) and served by the Go backend:

- `GET /v1/godot` — lists the Godot versions (id, title, ready, url)
- `GET /godot/<game>/` — the Godot web export, served with the
  `Cross-Origin-Opener-Policy` / `Cross-Origin-Embedder-Policy` headers the
  Godot 4 engine requires
- `/player` shows a "GODOT VERSIONS" section with Play buttons

The repo stays text-only (the GitHub connector corrupts binary uploads), so
compiled exports are **not committed**. `godot/web-exports.json` maps each
game to a zip of its web export; the server downloads and extracts it on
first request into `godot/<game>/web/` (gitignored).

To rebuild the exports:

```sh
# needs the 4.7.2 export templates installed (see ~/workspace/tools/godot-templates)
bash /tmp/export_web.sh   # or: godot --headless --path godot/<game> --export-release "Web" <out>/index.html
```

The zip URLs in `web-exports.json` are Hatch file-storage links and **expire**
(currently ~2 days). If a server ever reports the Godot version "still
downloading" after the links expire, re-upload the zips from
`~/workspace/godot-exports/` (`remote-storage upload-file`), update the URLs
in `godot/web-exports.json`, and push — the server picks them up on its next
pull. Already-downloaded copies keep working regardless.

## Verify

Each project has a headless self-test: a bot plays the real game loop and
asserts the key beats (level-ups, game-over payload). No errors expected.

```sh
GODOT=~/workspace/tools/godot/Godot_v4.7.2-stable_linux.x86_64
$GODOT --headless --path godot/bobos-rainbow -- --self-test
$GODOT --headless --path godot/ribbon-sky -- --self-test
$GODOT --headless --path godot/escape-injection -- --self-test
```

`--api=http://host:port` on any project enables the game-over leaderboard
POST against a running DoodleQuest server.
