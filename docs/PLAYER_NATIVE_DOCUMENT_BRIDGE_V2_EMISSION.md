# Trusted Pixi native document-epoch message emission v2 (generator phase)

Parent: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [native lifecycle #203](https://github.com/GavinDu/Hephaestus/issues/203).

## Why this is staged

The current shipping native WebKit host accepts only `schema:1` messages
with a JavaScript `sessionId` and an eight-ID retired-session window.
After sufficient reloads, queued old-document messages could be mistaken
for first events from the new session. A future native document challenge
must match the actual main-frame navigation, but forcing v2 from the
server before a companion native app rollout would silently break older
iPhone/iPad releases.

## Phase 1: trusted runtime producer (this PR)

The trusted bundled Pixi engine emits only two narrow native message
types: `player_metric` and `verified_run_available`. The latter is
attempted **only** after the backend returns explicit `verified: true`
and still requires independent host/backend verification.

The new pure `buildNativeBridgeMessage` chooses:
- If `window.__hephaestusNativeBridge` is **undefined**, the legacy
  schema-1 envelope is unchanged. Non-iOS browsers may have no WebKit
  handler; telemetry remains optional.
- If a native-installed object is present with **exact** `schema:2`
  and a 32-character lowercase-hex `documentNonce`, it emits schema-2
  plus that nonce for both event types.
- If the property is present but malformed, **send nothing**. Never
  downgrade to schema 1 on an invalid advertised v2 capability.
- Reserved privileged envelope fields cannot be overwritten by a
  metric payload; arbitrary native command types are not supported.
- The nonce is never sent to server analytics, URLs, log files or run
  uploads by this change.

Node source-extracted tests call the **shipping builder** and prove
schema-v1 compatibility, valid schema-v2 events, malformed nonce downgrade
rejection, replay-result mapping, and no new privileged native command.
The required Playability Kit npm test suite registers these tests.

## Phase 2: iOS native gate (NOT implemented by this PR)

Native must generate a cryptographically unpredictable 128-bit nonce per
**main-frame document epoch**, install it via trusted `WKUserScript` in
the page's content world **before** that navigation begins, and store
the expected nonce with the current pooled WKWebView attachment lease,
expected game/origin and committed document. It must rotate on true
document replacement and invalidate immediately on failure/termination,
rather than rotating after a late old-document message.

A v2-capable native build must compare exact nonce, main-frame, origin,
game/version and live navigation/credential epoch **before** accepting
`sessionId`, metric sequence or verified-result work. It must **not**
fall back to schema 1 once v2 is required for that runtime. Prewarmed
navigation, reload, retries, background, sign-out and 128+ queued stale
callbacks require Swift/WebKit tests. A nonce in page JS is only
anti-stale **document freshness**, not a proof of honest JavaScript:
trusted origin must not execute AI-authored scripts. Server-run claims
remain independently unverified until security issue #360 is resolved.

## Release and compatibility

This generator phase is a *producer compatibility prerequisite*, not
a deployed native security fix. Complete exact-head Go, PostgreSQL,
Chromium, WebKit, Playability Kit and cross-browser determinism jobs,
then adversarial game-engine architect review. Before enforcing native
v2, ship and test the native pre-navigation injector and reader against
a versioned trusted generator, with an explicit unsupported-runtime
failure instead of a silent legacy downgrade. No physical iPhone/iPad
50-cycle, GPU/thermal evidence is claimed. Do not close native #203
or Epic #202 after this phase alone.
