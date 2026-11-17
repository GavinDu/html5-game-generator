# Minimal Game IR browser bootstrap — Epic #202 / #478

## What this new mandatory test executes

The shipped `internal/api/web/pixi-runtime.js` is launched in the regular loopback-only game player under Playwright **Chromium and WebKit** using iPhone 390×844 and iPad 820×1180 viewports. The only test interception is the declarative `GET /v1/games/escape-from-injection/package` Game IR document and matching envelope version/orientation/asset list, replaced with a synthetic minimal v1.0 document with one inert entityless scene and ordered `game_start` actions `add_score(+3), add_score(-2)`. No first-party Pixi source, module import, game rule implementation, renderer, score variable, state, probe return, or browser rendering is altered.

The read-only `window.__dqTest.snapshot()` is created *after* the preinstalled Pixi engine executes the start event and submits its real canvas render. The test requires genuine 2+ frames, a progressed fixed simulation step, final score 1 exactly once, zero authored entities, not failed, not ended, not won, no result card, no pending input, and an additional subsequent simulation step with the same score. Browser reload repeats the same real package loader and runtime for a second cycle. Each viewport and cycle captures an original full-page screenshot; browser CI uploads four per browser plus a machine-readable report. A failure leaves actual browser logs and screenshots available; no silent skip or mock success.

## Explicit nonclaims

**DIAGNOSTIC ONLY.** The package request is replaced by a test-only Playwright route; its full IR digest is intentionally cleared. It is *not* a genuine published immutable package, authenticated user session, independent server-owned replay, real device/iPhone GPU/thermal benchmark, or trusted score. The browser's own score is not server authority, and the test does not submit or endorse `Verified` or leaderboard placement.

The parent [#469](https://github.com/GavinDu/html5-game-generator/issues/469) must still deliver authentic published-package browser-vs-Go conformance, exact immutable bytes/hash, independent Go reducer state, seed, ordered intermediate steps, restart and failure cases. [#428](https://github.com/GavinDu/html5-game-generator/issues/428) must implement a bounded fixed-tick semantic worker with genuine v2 trace. [#382](https://github.com/GavinDu/html5-game-generator/issues/382) must own final attestation. Native physical iPhone/iPad hardware and Hephaestus #202 release remain open.

## Rollback and acceptance

The entire change is one Playwright test script, one required workflow browser step and artifact upload per browser, plus this documentation. No production code or API changes. Review exact-head test source for whether all observations originate in real preinstalled Pixi, never fabricated screenshots or Playwright state injections. All six exact-head Go/race/PostgreSQL/Chromium/WebKit/determinism/Playability Kit jobs **must** pass before merging/closing #478.
