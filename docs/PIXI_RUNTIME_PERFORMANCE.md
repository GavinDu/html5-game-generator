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

## WebAudio session isolation and cancellation (Hephaestus Epic #202 / iOS #205)

The global runtime audio pool is now **session-owned** even when the page's WKWebView is recycled: an in-place restart invalidates a generation token, aborts in-flight protected audio fetches, clears queued sounds, stops currently playing buffer sources, drops decoded buffer references and closes its previous AudioContext. A `pagehide` event applies the same cleanup when the native host navigates off the game; background visibility suspends audio.

The cache key includes both the asset ID and its delivery URL. Two AI games may both name a sound `chime` without reusing each other's audio. Cache contents are still cleared at every restart to avoid stale version reuse.

Initial safety ceilings: **8 MiB compressed transfer per sound**, **24 MiB decoded PCM per buffer**, **32 MiB total decoded audio per session**, **32 simultaneous active sounds** and **32 queued sounds awaiting an unlock gesture**. Duplicate requests for the same sound share one in-flight decode, with **at most four concurrent audio fetch/decode jobs** and a **15-second transfer timeout**. The browser must report failed audio requests with their reason; Playability Kit must continue to fail on genuine asset errors. A budget rejection does not trigger an unbounded fallback fetch. If WebAudio is unavailable, sound is disabled rather than opening an unrestricted HTMLAudio URL. These are protective defaults, not measured target-device budgets. Browser audio decoding itself may allocate before its PCM size can be checked. Full app-level prewarm cancellation, Rapier world/texture release and 50-cycle iPhone/iPad memory/thermal evidence remain open under #205.

Verification is required through JS source/regression guards, Chromium/WebKit Playability, restart-in-place screenshots and actual WebKit device memory traces. Passing browser CI alone does **not** establish zero native memory leaks.
