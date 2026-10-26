
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
