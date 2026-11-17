# Real Pixi native bridge producer first-packet regression — Epic #202 / #484

This browser-only, source-preserving regression uses the **unmodified shipped Pixi engine and real checked-in inventory package** for game `escape-from-injection`. On Chromium and WebKit, Playwright installs a capture-only `window.webkit.messageHandlers.hephaestusRunResult.postMessage` sink at document start and selects explicit bridge configurations before any engine source is loaded. The test does not alter `buildNativeBridgeMessage`, `sendNativeMetric`, score, game rule IR, renderer, player probe or scene submissions.

- Valid v2 document A (`a` × 32 lowercase hex) on iPhone viewport 390×844 must produce **real** `first_scene_render_submit` with `schema=2`, current game, nonempty ASCII session, strictly increasing metric sequence, exact nonce, and a running fixed-step Pixi canvas.
- A fresh valid v2 document B (`b` × 32) on iPad viewport 820×1180 proves packets are generated with a new bridge nonce, not hardcoded to previous document.
- A malformed **present** v2 bridge must produce **no packets**, while actual Pixi continues to render. It must never silently downgrade to schema 1.
- An absent bridge simulates old browser/non-v2 native host compatibility and still emits schema 1 **without a v2 document nonce**. This case is not intended to be admissible in a future strict native host.
- Genuine page screenshots, source runtime frame/tick measurements and sanitized captured packet summaries are exported per browser. No credentials or unique user data are emitted. The screenshot is evidence of a browser canvas, not actual iPhone/iPad GPU composition.

## Senior game-engine-architect challenger / release boundary

The test-only `postMessage` sink is **not** a trusted WKScriptMessageHandler. A browser page can forge these packets, so real iOS must verify main-frame origin, schema, nonce, run session, credential epoch, active attachment/lease, parser budget and pool-owned prewarm transfer. Do not convert this producer test into server-owned replay attestation or authoritative verified scores.

Native [#286](https://github.com/GavinDu/Hephaestus/issues/286) remains OPEN: production still selects `PlayerNativeBridgeVersionGate(policy: .legacyCompatible)`; the strict `.requireV2` rollout requires confirmed deployed producer compatibility, first native callback and authentic published creator fixture on iPhone/iPad, full seven native exact-head CI, physical device evidence and explicit rollback. Poster handoff [#353](https://github.com/GavinDu/Hephaestus/issues/353) and physical compositor [#276](https://github.com/GavinDu/Hephaestus/issues/276) also remain open.

## Merge gate and rollback

All SIX generator exact-head Go/race/Postgres/Chromium/WebKit/cross-browser-determinism/Playability Kit jobs must PASS, including mandatory producer test in both browsers, before this **independent browser evidence child** can be closed. No skips, no fake metrics or browser-score writes, no paid Replit models. Rollback removes one Playwright test, its workflow step/artifacts and this documentation; production runtime and iOS behavior are unchanged.
