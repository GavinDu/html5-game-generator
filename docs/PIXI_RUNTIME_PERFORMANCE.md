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


## All-inventory cross-browser semantic replay (Epic #202 / #315)

The deterministic verification gate must cover **every shipped Game IR**
under `inventory/*/manifest.json`, not only Escape's lane runner. The
`inventory-semantic-replay-test.mjs` Playwright job discovers the canonical
game IDs dynamically, launches each *unmodified* package in Chromium and
WebKit at both iPhone and iPad portrait viewport sizes, seeds the trusted
interpreter identically, and advances it to exact virtual step 240. It
captures gameplay state and an unaltered screenshot; it does not compare
pixel hashes across GPU/browser engines.

The dependent `cross-browser-determinism` job downloads the independent
browser artifacts, requires complete/unique game+viewport coverage and
compares seeded tick, score, health, objective progress, variables, player
position and nondecorative gameplay entity positions to 0.001 logical
pixels. A missing game, duplicate profile, wrong tick, missing fixture or
single semantic outcome difference fails the workflow instead of weakening
tolerance. Local comparator unit tests explicitly reject these cases.

This supplements existing Escape 600-tick replay and quality-invariance
checks; it does **not** prove arbitrary touch/input traces, varied
30/60/120 Hz render pacing, thermal interruption semantics or physical
iOS/iPadOS WebKit conformance. All remain Epic #202 acceptance gates.

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

Audio URLs are additionally required to be same-origin before any authenticated fetch. The negative browser fixture mutates the Penguin sound URL to an external host and proves no request escapes.

## Active runtime entity cap (Epic #202 / #316)

The compiler already caps *authored* entities at 500 per scene, but that
does not constrain the number of live clones created through authored
`spawn_entity` rules over a long session. The installed browser engine
now sets a **512 active-entity ceiling** for its one live scene. Initial
tampered packages above the ceiling fail *before* Pixi/WebGL/Rapier
allocation. Dynamic `createEntity` checks the count *before* creating
any sprite, rigid body, collider or pointer listener. A tick that exceeds
this limit stops its ticker, reports a runtime-error diagnostic and shows
a non-winning, actionable message; it does not submit a fabricated
verified run or continue executing partial rules.

The Chromium/WebKit CI matrix modifies only a test response to declare
513 scene entities, checks the fail-closed player state and screenshots
on phone/tablet profiles, and preserves the entire original game-quality
suite. These are hard safety ceilings, not measured optimum concurrent
sprites for every iPhone/iPad. Dynamic spawn abuse, repeated recycle
behavior, per-tick event/action costs, joint creation and GPU texture
budgets still require profiling and typed preflight under #316.


## Fixed-tick animation completion signals (Epic #202 / #315 / #317)

Pixi AnimatedSprite instances previously subscribed to Ticker.shared:
their visual frames and `animation_complete` rule signals could advance
while a deterministic capture's semantic simulation was frozen or at a
different time on a 30/60/120 Hz renderer. A virtual clock alone does not
make such off-tick callbacks deterministic.

The trusted interpreter now sets `AnimatedSprite.autoUpdate=false` before
calling `play()`. The only advancement is `AnimatedSprite.update`
**inside `tickFrame(dtMs)`**, with exactly one Pixi logical frame of
`deltaTime=1` per 1/60 s simulation tick. A one-shot's completion
callback runs under the same fixed-tick rule/action budget; visual refresh
cannot generate an extra rule or outcome. This also freezes animation
during shell start, capture pause, hidden-page pause, and app interruption.

A synthetic Playwright fixture (separate from the immutable shipped games)
adds a two-frame, six-FPS one-shot to Escape's player and observes the
read-only animation frame and `animation_complete` rule marker at fixed
steps 0, 10, 19, 22 and 40. Chromium/WebKit test both iPhone/iPad
**portrait browser viewports**, a real 450ms semantic pause, nonwinning
behavior and per-case screenshots. Existing all-inventory seeded replay,
quality invariance, natural playability and Go tests remain required.

This addresses a specific renderer-driven semantic signal; it does not
make collision ordering, renderer-owned particle effects, pointer
normalization or real iOS GPU frame presentation deterministic.
`AnimatedSprite` API compatibility is tied to the trusted vendored Pixi
build and must be rerun when that dependency is upgraded. No native
bridge, Game IR version, or user-supplied script capability is introduced.
