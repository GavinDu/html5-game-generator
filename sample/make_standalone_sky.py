#!/usr/bin/env python3
"""Build a standalone single-file HTML copy of the 飘带补天 Ribbon Mends
the Sky sample game.

Downloads the published bundle (manifest, game.js, sprites, music) from a
running DoodleQuest server and inlines everything as data URIs into a copy
of the web player. The result opens in any browser with no server needed.

Usage:
    ./sample/make_standalone_sky.py [base_url] [output]
    ./sample/make_standalone_sky.py http://localhost:8080 sample/ribbon-mends-sky-standalone.html
"""
import base64
import json
import re
import sys
import urllib.request

GAME_ID = "sample-ribbon-sky"
TITLE = "飘带补天 Ribbon Mends the Sky"
SPRITE_IDS = ["bg", "hero", "demon", "hole", "hole_closed", "shard", "spark"]


def get(url):
    with urllib.request.urlopen(url) as r:
        return r.read()


def mime_for(src):
    return "data:image/jpeg;base64," if src.lower().endswith((".jpg", ".jpeg")) \
        else "data:image/png;base64,"


def main():
    base = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:8080"
    out = sys.argv[2] if len(sys.argv) > 2 else "sample/ribbon-mends-sky-standalone.html"
    bundle = base.rstrip("/") + f"/v1/games/{GAME_ID}/bundle"

    manifest = json.loads(get(bundle).decode())
    code = get(bundle + "/game.js").decode()
    sprites = {}
    for sid in SPRITE_IDS:
        for src in [s["src"] for s in manifest["assets"]["sprites"] if s["id"] == sid]:
            raw = get(bundle + "/" + src)
            sprites[sid] = mime_for(src) + base64.b64encode(raw).decode()
    music_raw = get(bundle + "/" + manifest["assets"]["music"]["src"])
    music = "data:audio/wav;base64," + base64.b64encode(music_raw).decode()

    standalone = {
        "manifest": {"name": manifest["title"],
                     "assets": {"sprites": [{"id": s, "src": f"sprites/{s}"} for s in SPRITE_IDS]}},
        "code": code,
        "sprites": sprites,
        "music": music,
    }

    with open("internal/api/web/player.html") as f:
        html = f.read()

    data_script = ("<script>\nvar STANDALONE = " + json.dumps(standalone)
                   + ";\nvar bundle = STANDALONE;\n</script>\n")
    html = html.replace("<script>", data_script + "<script>", 1)
    html = html.replace("img.src = base + '/' + sp.src;",
                        "img.src = STANDALONE.sprites[sp.id];")
    html = html.replace("musicEl = new Audio(bundleBase + '/' + src);",
                        "musicEl = new Audio(bundle.music);")
    html = html.replace(
        """  var base = API + '/v1/games/' + gameId + '/bundle';
  fetch(base).then(function (r) {
    if (!r.ok) throw new Error('bundle HTTP ' + r.status);
    return r.json();
  }).then(function (manifest) {""",
        "  Promise.resolve(STANDALONE.manifest).then(function (manifest) {")
    html = html.replace(
        """      return fetch(base + '/game.js').then(function (r) {
        if (!r.ok) throw new Error('game.js HTTP ' + r.status);
        return r.text();
      }).then(function (code) {""",
        "      return Promise.resolve(STANDALONE.code).then(function (code) {")
    html = html.replace("var Shim = makeShim(base, imgs);",
                        "var Shim = makeShim(null, imgs);")
    html = html.replace("loadGames();", "playGame('standalone');")
    html = html.replace(
        "document.getElementById('back').onclick = function () { cancelAnimationFrame(rafId); show('list'); };",
        "document.getElementById('back').onclick = function () { playGame('standalone'); };")
    html = html.replace(
        "document.getElementById('togames').onclick = function () { show('list'); };",
        "document.getElementById('togames').onclick = function () { playGame('standalone'); };")
    m = re.search(r"function loadGames\(\) \{.*?\n\}\n\n", html, re.S)
    html = html[:m.start()] + html[m.end():]
    html = html.replace("<title>DoodleQuest Player</title>",
                        f"<title>{TITLE} — DoodleQuest</title>")
    html = html.replace("<h1>🎮 DoodleQuest Player</h1>",
                        f"<h1>🎀 {TITLE}</h1>")

    with open(out, "w") as f:
        f.write(html)
    print(f"wrote {out} ({len(html)} bytes)")


if __name__ == "__main__":
    main()
