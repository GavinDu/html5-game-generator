# Authoritative run input completeness (Epic #202 / #315 / #316)

## Existing mismatch
The Go API rejects leaderboard run submissions with more than 2,000 replay input events. The browser runtime previously retained only the most recent 2,000 normal input events, silently discarding the crucial beginning of the trace, while its sampled held-pointer events had no shared bound. Neither a truncated prefix nor an unbounded capture is acceptable evidence for deterministic score verification.

## Shipping interpreter change
`runtime-replay-input.mjs` is trusted and served only by the Go host (JavaScript MIME, `nosniff`), not declared or downloaded from a game package. `createRunInputRecorder()` holds up to `MAX_VERIFIABLE_RUN_INPUTS=2000` ordered, tick-stamped events. Every normal `noteInput` and pointer/held-pointer sample now calls `record()` rather than mutating the array. Invalid or excessive events permanently mark the current run `complete=false`; the first 2,000 are retained intact for diagnosis, but are never submitted as a complete replay.

On game completion, a *complete* run follows existing server submission and backend-verified native result behavior. An *incomplete* run still records its local win/score and local best, but neither submits a leaderboard run nor sends any native `verified_run_available` message. The score subtitle explicitly says **Not leaderboard-verified: replay input limit reached. Play again for a new attempt.** Restarting allocates a new recorder, not a carried-over overflow flag. The separate replay-verification path remains intact.

This change does not alter rule execution, simulation ticks, input handling or physics; the recorder only decides if the collected input history is complete enough to *claim* replayability. The input types are explicitly allowlisted to the existing runtime forms `pointer`, `pointerup`, `key`, `keyup`, and `entity-pointer`. New input capabilities must update that list together with the generator and replay verifier.

## Test/acceptance
Node test: 2,000 accepted; 2,001 rejected; mixed keyboard/held pointer preserved in order; 40,000 further held samples bounded; malformed tick/type/detail not submitted; new session reset; production-path static assertions proving one recorder, no old mutating `push`/`splice`, no submission of incomplete runs. Go route test: trusted code, MIME, nosniff, constant. Full Go validation is required at exact PR head: Go tests, PostgreSQL, Chromium, WebKit, Playability Kit, cross-browser determinism. Exact screenshot of the overflow result message is a **separate browser fixture still required** before the full #315 issue can close.

## Outstanding architect concerns
- `play.pendingInputs` (200ms dead-input diagnostics) can grow while the display clock is suspended; independently bound it and mark incomplete diagnostics rather than asserting missing samples are real dead inputs.
- 2,000 events is a compatibility limit, **not** support for long-running high-frequency pointer games. A versioned append-only streaming/chunked replay protocol with canonical tick range/hashes and server verification is needed for longer sessions, without silently increasing the API limit.
- Hardware memory and thermal measurements, deterministic fingerprints for other game families, and full phone/tablet lifecycle evidence are still open; do not close #315, #316 or Epic #202 based on this fix alone.
