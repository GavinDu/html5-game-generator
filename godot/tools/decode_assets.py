#!/usr/bin/env python3
"""Decode committed .b64 sprite text into real PNG/JPG files for the Godot projects.

The repo never stores raw binary (GitHub connector file tools corrupt it), so
the Godot projects reference assets/ images that are materialized by this script
from the text files already committed under inventory/<id>/assets/.

Usage:
    python3 godot/tools/decode_assets.py
"""
import base64
import os

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))

# godot project dir -> (inventory id, [asset ids])
GAMES = {
    "bobos-rainbow": ("13ui1k", ["bear", "kitten", "heart", "heart_blue", "heart_gold",
                                 "band0", "band1", "band2", "band3", "band4", "band5"]),
    "ribbon-sky": ("u46ex5", ["bg", "hero", "demon", "hole", "hole_closed", "shard", "spark"]),
    "escape-injection": ("2uowg9", ["blob", "chaser", "syringe", "star", "bg0", "bg1", "bg2"]),
}

EXT = {"bg": ".jpg"}  # sky backdrop is a JPEG; everything else is PNG


def main():
    n = 0
    for game, (inv, ids) in GAMES.items():
        outdir = os.path.join(ROOT, "godot", game, "assets")
        os.makedirs(outdir, exist_ok=True)
        for aid in ids:
            ext = EXT.get(aid, ".png")
            src = os.path.join(ROOT, "inventory", inv, "assets", aid + ext + ".b64")
            if not os.path.exists(src):
                # Procedural sprites (hearts, rainbow bands) are drawn natively
                # in Godot — no binary to decode.
                print(f"  skip {game}/{aid} (procedural, drawn in-engine)")
                continue
            with open(src) as f:
                raw = base64.b64decode(f.read())
            with open(os.path.join(outdir, aid + ext), "wb") as f:
                f.write(raw)
            n += 1
    print(f"decoded {n} assets")


if __name__ == "__main__":
    main()
