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

## Pure deterministic RNG module (Epic #202 / generator #317)

The installed Game IR interpreter imports `/web/runtime-seeded-rng.mjs`, served by
the trusted Go runtime with a strict JavaScript MIME type and `nosniff`.
This isolated stateful Mulberry32 primitive has the **same exact bit/float
sequence** as the former inline generator, including the original zero-seed
normalization and 32-bit wrap. Existing gameplay seeding and legacy run
replays are not reinterpreted just because code moved into a new module.

The new `runtime-seeded-rng.test.mjs` checks 1,000 reference draws for each
of seven boundary seeds and verifies isolated instances/reseed behavior.
The ordinary Go/Node/Chromium/WebKit Playability and cross-browser replay
checks remain mandatory for launch behavior. Adding one trusted module
request may impact cold-start latency; compare device timings before
adopting a broader micro-module strategy.

**Not yet resolved:** visual particles/shake and semantic gameplay still
consume a shared stream inside the interpreter. Their coupling is
undesirable, but splitting streams *changes existing seeded outcomes* and
must be introduced behind a new versioned replay/capability contract with
fixture migration rather than silently reinterpreting prior leaderboard
runs. Physics/event state, Pixi rendering and WebAudio remain in the main
interpreter. This is a compatibility-safe **first extraction**, not completion
of #317 or full #315 determinism.
