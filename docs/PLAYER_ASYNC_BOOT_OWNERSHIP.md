# Exclusive asynchronous game bootstrap — Epic #202 / generator #317

## Source-confirmed failure and risk

The shipping Pixi runtime's `run(id)` called `apiFetch` for replay/package,
then `await app.init()`, then awaited up to six concurrent image workers
and `RAPIER.init()`. The old run was only revocable through
`activeSession.abort` **after** `app.init()` completed. Two starts (rapid
Play Again, script-driven restart, retry after failed boot) could overlap
before `activeSession` existed, allowing an older promise to resume later
and install its stage, session callbacks, probe snapshots or success signals
over a newer attempt. Cancelling old timers alone (#365) does not fix this.

## New runtime ownership boundary

1. A **single** `createGameRunAttemptGate` synchronously aborts the previous
   attempt and mints a new `AbortController` at the very start of `run`,
   before replay/package fetch or other awaits.
2. `awaitGameRunAttempt(promise, attempt)` checks current ownership on both
   fulfillment and rejection. An old promise produces a typed local
   `StaleGameRunError`, not a current-sheet error. Replay/package HTTP
   requests carry the active attempt's AbortSignal.
3. An abandoned or failed Pixi `app.init` destroys only that attempt's
   partly initialized app. Its asynchronous continuation cannot append
   the old canvas to the current stage.
4. Image transfer requests chain their 15-second timeout controller to the
   session abort; cancellation is checked after fetch/body, hashing/decode,
   and immediately before texture creation or storing it in the attempt's
   map. The original six-worker bound, content hash, size/dimension and
   pixel budgets remain unchanged. A discarded bootstrap releases its
   private previously decoded textures.
5. The Rapier initialization await is owned by the same attempt. A failed
   or superseded asset/physics bootstrap releases its own renderer, not
   the successor's `activeSession`.
   On normal in-page Play Again, the retired session explicitly disposes
   its *run-private* decoded textures after Pixi destroys its scene, while
   leaving Pixi global/shared engine textures alone. Disposal clears the
   private map first and is safe if called twice or one texture already
   disappeared.
6. Restart and initial boot entry-point catch handlers discard only the
   typed superseded-attempt error; genuine package/HTTP/render failures
   retain visible existing error handling.

## Regression / challenger matrix

- A pure gate extracted from the **actual shipping JS source** is executed
  by Node tests through 128 overlapping begins. Previous controllers
  immediately become aborted and can never assert ownership again.
- Synthetic delayed promises prove that an old successful fetch arriving
  after a newer attempt cannot become the latest game, and that an
  aborted old network request cannot overwrite current UI errors.
- A genuine error on the current attempt remains visible. Static source
  contracts pin guard points for package, Pixi init, image workers,
  texture upload, Rapier and AbortSignal transfer linkage.
- A separate executable test runs 128 generations of private image texture
  ownership and checks that aliased textures are retired once, retries are
  idempotent, and failure disposing one texture cannot retain other images.
- Required npm tests include the new fixture. The old bounded-image-worker
  test was updated to assert the worker pool **inside** the ownership wrapper
  rather than dropping its concurrency check.

## Senior game-engine-architect challenge / remaining work

This is a high-value fail-closed bootstrap ownership slice, not full
lifecycle proof. Pixi/Rapier shared initialization and error teardown still
need 100+ **real Chromium/WebKit** rapid-retry, forced delayed HTTP, image
decode and WebContent-termination evidence; the Node tests model ownership,
not device GPU behavior. Audit remaining asynchronous sound playback,
post-boot result requests and texture/physics teardown during normal session
completion; handle all synchronous post-Rapier setup failures with a unified
`try/finally` owner, ideally after splitting engine adapters into modules.
A dedicated, versioned native per-document nonce/credential lease is still
required on the iOS host. Deterministic server replay attestation is a
separate security blocker (#360). Do not close generator #317 or Epic #202.

**Release requirement:** upstream #365 must merge independently with all
six checks, then this stacked PR must be reparented to main, inspected at
its new head and rerun Go, PostgreSQL, Chromium, WebKit, Playability Kit
and cross-browser determinism before merge. iPhone/iPad physical GPU,
memory and thermal acceptance remains separate.
