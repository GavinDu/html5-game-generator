# Session-owned probe diagnostics — Epic #202 / generator #317

## Audited failure

The trusted Pixi game runtime previously called `setInterval(emitProbe, 2000)`
after first renderer submission in probe mode without retaining the handle or
clearing it on in-page Play Again. `restartGame()` aborts the previous
`activeSession` and destroys its Pixi app, but the old interval closure
continues retaining that run's state and can keep reporting obsolete probe
scores, entities and frame metrics. The prior `window.__dqTest` snapshot also
remained reachable between old-session teardown and new-session setup.

## Shipping incremental change

- Bind the probe interval to the existing session's `AbortController`, with
  a one-shot abort listener clearing the interval. Already-aborted async
  bootstrap attempts cannot create a new diagnostic timer. No timer in normal play.
- Associate the read-only `window.__dqTest` interface with its creating
  session and clear it only if that session still owns the global reference.
  Already-aborted attempts cannot expose a stale global probe API.
  Never remove a successor's probe object on late teardown.
- Retain the existing 2-second probe reporting cadence and immediate initial
  report; this is strictly a lifecycle fix, not a gameplay-semantic change.
- Test the actual installed runtime's timer installer in a controlled Node
  context through 64 sequential start/abort cycles, including duplicate abort,
  and assert that ordinary gameplay creates no interval. Source contract
  checks guard the probe-API identity check.
- Register the new test in the explicit npm test list used by Actions, rather
  than adding an unexecuted test file.

## Senior game-engine architect challenger / remaining risk

This guards the *diagnostic* timer and probe object on completed sessions; it
does not prove that in-flight asynchronous boot is exclusive. A second
`run()` can begin while the first is still awaiting package JSON, Pixi
`app.init()`, image fetch/decode or Rapier init, before the prior run has
committed `activeSession`. Current teardown only aborts `activeSession`
once assigned. Two concurrent runs may therefore race stage ownership, sound,
asset allocation and stale error UI. The next design slice should establish
an attempt epoch or abort ownership **before the first await**, propagate its
signal through package/asset fetches, check identity after every awaited
allocation, destroy stale Pixi resources, and suppress stale-run UI errors.
Test delayed first boot + 100 rapid restarts with forced out-of-order
resolution in Chromium and WebKit. Do not paper over this with timer cleanup.

Additional remaining work: DOM/Pixi/Rapier adapter teardown, resource ownership
for every texture/audio/physics allocation, native WKWebView document nonce,
server-run attestation, and physical iPhone/iPad GPU/memory/thermal measurement.

**Release gates:** exact-head Go, PostgreSQL, Chromium, WebKit, Playability Kit
and cross-browser determinism all complete successfully; inspect evidence and
review before merging. No physical-device or full #317/#202 closure claim.
