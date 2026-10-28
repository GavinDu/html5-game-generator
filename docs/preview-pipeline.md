
## Headless render step

`tools/playtest/render-preview.mjs` replays a stored run in Chromium or WebKit, captures a fixed 12-fps sequence for six seconds, writes a muted H.264 MP4 (`yuv420p`, `+faststart`), and saves the midpoint frame as the poster. Example:

```sh
node tools/playtest/render-preview.mjs \
  --base http://localhost:8080 \
  --game GAME_ID \
  --run RUN_ID \
  --out /tmp/preview.mp4 \
  --poster /tmp/poster.png
```

The renderer uses a fixed viewport of 420×900 CSS pixels and a fixed frame interval. If ffmpeg encoding fails after frame capture, the poster remains available and the command reports `clip_fallback: true`; this is intentionally non-blocking for poster-only preview publication. The command prints JSON containing frame count, duration, browser, output paths, and fallback state so the media-delivery decision can log render time and clip size around the invocation.


## Deterministic-render verification

Each render result includes `frame_sha256`, an ordered SHA-256 digest for every captured PNG frame. Run the same recorded run twice with identical runtime build, browser engine/version, viewport, options, and assets; compare the two `frame_sha256` arrays to detect visual divergence before publishing a clip. The digest is for frame-by-frame validation, not a guarantee of cross-browser or cross-version pixel identity. Poster-only fallback output includes hashes for all frames successfully captured before failure.


To compare two renderer JSON outputs, save them as `render-a.json` and `render-b.json`, then run:

```sh
node tools/playtest/compare-preview-hashes.mjs --first render-a.json --second render-b.json
```

Exit code is `0` for identical ordered frame hashes, `1` for a visual/frame-count mismatch, and `2` for malformed input or a missing file. This is suitable as a CI gate when both renders use the same browser/runtime environment.
