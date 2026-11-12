# PixiJS runtime performance and resilience

## Changes in this iteration
- Image assets now load through a bounded six-worker pool instead of a serial await loop. Independent requests overlap without flooding mobile Safari/WKWebView.
- Individual image loads have a 15-second timeout and fail independently. Missing optional images are logged and do not hold startup forever.
- The developer log is capped at 50 lines in both its backing buffer and DOM; previously only the buffer was capped while visible text grew for the whole session.

## Runtime invariants to preserve
- Keep the fixed 60 Hz simulation step and deterministic seed + input-trace behavior used by replay verification.
- Keep visibility-aware pause/resume and clamp catch-up work after backgrounding.
- Keep the native iOS host bridgeless; do not add a script-message bridge to control the runtime.
- Distinguish failed required assets from optional decoration in a future IR/package revision. Until the package contract marks criticality, current behavior logs asset failures and continues.
- Keep telemetry bounded and ensure telemetry failure never interrupts gameplay.

## Measurements to add / watch
- Time to first rendered frame, time to interactive, and time to first meaningful input.
- Package JSON fetch, image decode, physics initialization, and scene-build durations separately.
- Asset count, total asset bytes, failed/timed-out assets, and decoded texture dimensions.
- p50/p95 frame time, long-frame count, visible-play FPS, and fixed-step catch-up steps.
- Memory/texture behavior across repeated game open → close → open cycles.
- Context-loss recovery and process-termination recovery on iOS.

## Follow-up optimization order
1. Measure whether network, image decode, Rapier initialization, scene construction, or frame execution dominates startup.
2. Add package-side image dimension/byte budgets and generated thumbnail variants.
3. Reuse decoded assets only through a bounded cache with explicit ownership and eviction; do not introduce a global texture cache without lifecycle tests.
4. Consider culling/off-screen update suppression only where it cannot alter deterministic gameplay or collision outcomes.
5. Keep low-quality mode as presentation/performance degradation only; do not remove core mechanics, plot, strategy, or playability.

## Verification
- Run the existing Playwright browser matrix and deterministic replay checks against the three inventory games.
- Compare cold-start and repeat-start metrics before and after this change on Chromium and iOS WKWebView.
- Confirm a missing or stalled image no longer blocks other image loads and that the game reaches its normal validation verdict.

## P0 asset-safety backstop (Epic #202 / generator #316)

The browser runtime now caps image *transfer* at **8 MiB per asset**, including streamed responses without a trustworthy `Content-Length`, with its 15-second fetch timeout active until the response body is consumed. It also rejects images over **4096 pixels on either side**, **16 megapixels per texture**, or **32 megapixels aggregate** before calling `PIXI.Texture.from`. A rejected required image fails load, rather than silently disappearing or allocating an unbounded GPU texture.

These values are initial **security/resource ceilings**, not evidence-backed iPhone/iPad performance budgets. The browser may already allocate decoded pixels to complete `Image.onload`; true pre-decode protection requires publish-time header/dimension inspection and target-device memory tests. Audio bytes/buffers and other scene resource classes are not covered by these image-only caps. The generator and iOS capability manifest must eventually enforce equivalent budgets before publishing, with explicit user-facing error categories.

Verification: `tools/playtest/runtime-image-budget.test.mjs` checks the guard ordering and a Playwright negative fixture injects an oversized required image in Chromium and WebKit;  Go, Chromium/WebKit, Playability and physical-device checks remain required. Failures must not be treated as successful/partial gameplay.


## Session-owned sound decode and offline stress (Epic #202 / native #205)

Playability Kit deliberately interrupts connectivity after a burst of
interactions. In the prior bounded-audio candidate, Penguin's first chime
request could be aborted inside this offline window, producing
`net::ERR_ABORTED` and a **blocking** release failure. Do not hide this
failure by weakening the asset-quality gate.

The runtime now starts an *optional*, nonblocking predecode for at most the
two most frequently referenced `play_sound` assets after package validation
and image loading. It does not play them, block rendering or add new
gameplay actions. The decoder retains an asset+URL scoped cache with up to
four simultaneous protected same-origin fetches, 8 MiB compressed-byte
transfer per sound, 24 MiB decoded PCM per buffer, 32 MiB decoded PCM total,
32 concurrent active sound sources and 32 queued gestures. Every fetch
has a 15-second transfer deadline. The same WebView cannot carry decoded
sounds or stale asynchronous callbacks between games: restart and pagehide
clear the cache, pending jobs, source playback, audio context and fetches.

Read-only probe mode reports which sound IDs are successfully decoded.
Both Chromium and WebKit iPhone/iPad viewport CI require Penguin `chime`
to finish decoding **before** a simulated offline interval, stay cached
during the offline interval, and have no failed chime requests. Screenshots
and JSON evidence are uploaded. No audio permissions or arbitrary native
bridge are added. Audio remains an optional presentation effect; a
transient sound should never change authoritative game outcomes.

**Pending device/architecture requirements:** Page background behavior,
real iOS WebAudio unlocking, interruption/resume, decode overhead and
50+ open-close cycles with WebContent memory and thermal baselines.
Speculative predecode is a network/memory tradeoff, not a blanket startup
optimization; if it harms physical cold-start cohorts, redesign to a
user-gesture-triggered bounded decode that still survives offline play.
