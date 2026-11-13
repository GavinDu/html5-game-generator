# Runtime interruption and resume ownership (Epic #202 / generator #318, native #205)

## Invariant

A gameplay ticker is eligible to run **only** after the shell has begun, while the
document is visible, the authored orientation is satisfied, the WebGL context
is available, the result is neither terminal nor failed, and the renderer has
not faulted. Explicit deterministic capture controls never auto-resume on
visibility/orientation/restore. A semantic step retains its fixed 60 Hz cadence;
these gates do not change the seed, game IR or tick rules.

The trusted `shouldRunSemanticTicker` function in
`internal/api/web/runtime-work-budget.js` owns this eligibility decision. The
shipping Pixi interpreter calls one resume function rather than independently
restarting on page visibility, orientation or GPU restoration.

## Cancel-on-interruption

- Document hidden/blur: cancel pointer contact, held keyboard actions and
  unconsumed semantic commands. Stop the ticker; no hidden simulation.
- Pointer cancel: revoke the gesture and pending commands. Unexpected capture
  loss **while still held** is also cancellation, but loss of implicit capture
  **after a normal pointerup** must preserve a quick tap queued for the next
  semantic tick. The browser test proves both paths.
- Invalid orientation: cancel input and stop. Correct orientation can resume
  only if all other gates pass.
- WebGL context loss: stop simulation while the renderer cannot present; cancel
  pending commands. The existing Pixi restore handler remains responsible for
  graphics reinitialization. A restoration event rechecks the *full* predicate,
  and cannot revive a terminal, failed, hidden or capture-only run.
- Session teardown: `AbortController` owns both context listeners alongside
  other player listeners, so old sessions cannot resume their successor.

## Senior game-engine architect challenge

**Source-level acceptance:** clear, finite state guards prevent the
visibility-induced terminal resurrection and stale input on interruption.
Node truth-table coverage and real Chromium/WebKit input-cancellation checks
must be green before merge. The browser check rotates phone and tablet
viewports and proves no fixed semantic tick advances while blocked.

**Not yet proof:** GPU restoration may still require GPU resource reupload
or a full retry on particular iOS/WKWebView builds; `webglcontextrestored`
does not establish compositor presentation. Real iPhone/iPad WebContent
termination, 50+ foreground/rotation cycles, GPU memory/thermal profiles
and native #203 lifecycle authority remain gates in Epic #202. No second
JS/native control bridge or content-provided executable code was added.
