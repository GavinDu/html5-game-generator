# Game inventory

Every shipped game lives in its own folder here, named by a unique
6-character string (lowercase letters + digits):

```
inventory/
  13ui1k/                  # Bobo's Rainbow
    manifest.json          # game metadata + asset list (required)
    game.js                # GameShim v1 game code (required)
    assets/
      bear.png.b64         # sprites as base64 text (binary-safe for git)
      kitten.png.b64
    plot.md                # design doc (dev)
    make_standalone.py     # standalone single-file builder (dev)
    verify.js              # headless playtest (dev, optional)
  u46ex5/                  # 飘带补天 Ribbon Mends the Sky
    ...
```

At server start `internal/inventory` loads every `<6-char-id>/` folder:
it reads `manifest.json`, decodes the `assets/*.b64` sprites (or renders the
`procedural` ones via `internal/media`), synthesizes the music loop for the
manifest's `music.mood`, and registers the game. Loading is idempotent.

## Adding a game

1. Generate a fresh id, e.g. `python3 -c "import secrets,string;print(''.join(secrets.choice(string.ascii_lowercase+string.digits) for _ in range(6)))"`
2. Create `inventory/<id>/` with `manifest.json`, `game.js`, and `assets/`.
3. Copy the manifest shape from an existing game. A sprite is either
   `"file": "assets/name.png.b64"` (base64 text) or
   `"procedural": "heart:#RRGGBB"` / `"procedural": "band:0..5"`.
4. Restart the server — the game appears in `/v1/feed` and `/player`.

Folders not matching `^[a-z0-9]{6}$` are ignored. A duplicate `game_id`
across folders is a startup error.
