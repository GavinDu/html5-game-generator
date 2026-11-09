
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


## Repeat-render determinism check

Run `node tools/playtest/verify-preview-determinism.mjs --base URL --game GAME_ID --run RECORDED_RUN_ID` against the same recorded run twice. The command fails closed if either render falls back to poster-only output, omits frame hashes, or any frame hash differs; temporary media is removed after the check.

## Native-orientation determinism (2026-11-09)

`verify-preview-determinism.mjs` no longer forces a portrait `420×900`
viewport when the caller supplies neither dimension. It allows
`render-preview.mjs` to select the canonical Game IR orientation and
records the actual render dimensions. Callers can still explicitly set **both**
`--width` and `--height` to exercise a specific device profile; passing
only one dimension or an invalid size fails before rendering. This is
essential for landscape and tablet games and prevents a false "preview
cannot render" signal from a portrait-only test harness. A successful
hash match is still only a technical repeatability result, not proof that
the clip represents an approved playtest or is ready for distribution.

## Gameplay-first capture windows (2026-11-09)

A six-second clip beginning at simulation step zero can be dominated by an introductory story panel, even if a later gameplay screenshot is an acceptable poster. The renderer now accepts `--capture-start-ms N` (integer, 0–30,000; default 0) to begin the **recorded replay's deterministic 60 Hz simulation** at the reviewed offset. It does not modify the underlying Game IR, alter the recording, or trust a wall-clock sleep as a gameplay clock.

For a reviewed story intro that genuinely clears at seven simulated seconds, use:

```sh
node tools/playtest/render-preview.mjs --base http://localhost:8080 \\
  --game GAME_ID --run RECORDED_RUN_ID --capture-start-ms 7000 \\
  --out /tmp/gameplay.mp4 --poster /tmp/gameplay.png
node tools/playtest/verify-preview-determinism.mjs --base http://localhost:8080 \\
  --game GAME_ID --run RECORDED_RUN_ID --capture-start-ms 7000
```

The output includes `capture_start_ms` and the actual `capture_start_step` alongside ordered frame hashes. A failed replay, inaccessible game, or unreachable requested simulation step must fail rather than fabricate footage. Choosing a capture offset is **not** automatic authorization to publish: the candidate still requires inspection of actual screenshot frames, game-version/IR-hash matching, source rights, playtest completion where required, safety review, and independent delivery verification. An offset that skips a legitimate outcome can still be an unhelpful or misleading clip and must be rejected in review.

## Screenshot candidate session provenance (2026-11-09)

A screenshot captured after an introductory animation is not necessarily a
frame from the *winning* recorded playthrough. In Penguin, a guided player can
reach twelve hearts before the seven-second introduction clears. The quality
worker therefore captures `intro-cleared.png` in a second, independent **real
browser game session** if the initial session already ended.

`prepare-preview-fixtures.mjs` requires the successful guided win, a
legitimate still-running seven-second clearance, and the screenshot in the
quality artifact. Its candidate `preview-fixture.json` now records
`screenshot_session` (`guided-play` or `independent-real-browser`),
`screenshot_simulated_seconds`, and `screenshot_objective_progress`.
Reviewers must preserve that distinction; a supplemental screenshot must
**not** be labeled as a frame from the deterministic winning run. No
candidate may be automatically approved or published from the test verdict
alone. Version/IR hash, rights, moderation, actual clip replay and native
delivery checks remain independent gates.
