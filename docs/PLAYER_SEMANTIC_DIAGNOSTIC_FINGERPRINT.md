# Semantic diagnostic fingerprint v1 — Epic #202 / generator #315

## What changed in trusted runtime

The PixiJS player already executes rules at a fixed 60 Hz and exposes
read-only probe-mode snapshots to the Playability Kit. Earlier snapshots
had positions, health and score but **no canonical ordering or compact
semantic state signature**. Comparing only screenshots or terminal scores
can miss a mid-run divergence that later converges.

`runtime-semantic-fingerprint.mjs` is a pure, independently importable
module that normalizes a *probe-only* snapshot at a specific semantic tick.
Its canonical JSON includes fixed tick, seeded RNG stream position,
score/health/objective/depth, terminal flags, sorted scalar variables,
timer map, sorted rule/region/collection state, held keyboard/pointer state,
pending semantic input count, and sorted active entity identities,
definition/kind, logical position/size and closed state.

- Explicit **UTF-16 lexical key/entity ordering** avoids locale-dependent
  sorting; numeric logical values are finite and quantized to 0.001 units.
- The snapshot bounds active entities at 512, map/set fields at 512,
  coordinates and pending inputs. Duplicate entity IDs or nonfinite
  game-semantic state fail closed rather than producing misleading parity.
- The output includes a compact **FNV-1a 64-bit diagnostic hash** of
  canonical JSON. The hash is collision-prone and **never cryptographic
  verification**, an asset signature, a leaderboard authority or proof
  of execution.
- Excludes Pixi view alpha, animation frame, GPU/canvas state, wall-clock
  frame, camera/presentation and other nonsemantic rendering effects.
  Real semantic movement, rule-state, score, seed position or terminal
  differences change its canonical representation.
- Integrated **only** into the existing `?probe=1` read-only
  `window.__dqTest.snapshot()`; ordinary gameplay, Game IR contract,
  native message schema, physics tick, score and assets are unchanged.
- Registered Node tests check insertion-order invariance, visual-only
  changes, meaningful semantic changes, invalid bounds and duplicate
  IDs, and that shipping source does not send this hash as a native
  performance metric.

## Browser integration failure and source fix

The first exact-head [Chromium job #43494396151](https://github.com/GavinDu/html5-game-generator/actions/runs/43494396151) **FAILED** at the live feed smoke test waiting for `.game-card`; later Playability checks were cancelled/skipped. Node unit tests had passed. The code audit established that the shipping Go HTTP server uses an explicit embedded-module allowlist: adding a JavaScript `import` alone leaves the new `/web/runtime-semantic-fingerprint.mjs` URL as an explicit **404**. Because that import is parsed before the feed page initializes, the feed cannot render. This is an actual integration defect, not a flaky screenshot assertion.

The fix adds the module to `api.go`'s `//go:embed` set and explicit route with JavaScript MIME and `nosniff`. A new Go HTTP regression proves trusted source is served, the shipping player imports it, and unknown authored module URLs remain **404**. No wildcard file server or dynamic executable code was introduced. The initial failed run is retained, and only the new exact head's full six-job success can permit a merge.

## Senior game-engine architect challenger / what remains

**This does not finish #315.** It is diagnostic evidence from the
*existing browser-owned simulation*, not an independent semantic core.
The same runtime still owns Pixi/Rapier, and a browser client could
fabricate the hash. FNV64 and quantization can hide collisions/tiny
differences; full attestation #360 requires a server-executed model,
canonical cryptographic SHA-256, immutable IR+assets+runtime version,
fixed seed/input grammar and authenticated pending→verified/rejected
transaction.

Next verification layer should capture the fingerprint at identical
fixed ticks in **real Chromium and WebKit** for every inventory game
plus at least three synthetic genres, running the same seed+tick input
trace at fast and slow display frame pacing and low/high rendering
quality. Compare the **full canonical representation** (or SHA-256
over it) rather than trusting FNV alone, attach raw snapshots and
terminal screenshots, and investigate the first divergent tick.
Also benchmark high-speed swept overlap/collision and GPU loss and
resume semantics without gameplay advancement. Per-device iPhone/iPad
hardware first-composited frame, p50/p95, memory and thermal evidence
are separate Epic #202 DoD gates.

**Release:** exact-head Go, PostgreSQL, Chromium, WebKit, Playability Kit
and cross-browser determinism required. A source review or green Node
unit test alone is not merge approval. Do not close #315 or #202 on
this slice.
