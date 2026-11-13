# Runtime run-verification trust boundary — Epic #202, generator #315/#316

**Status:** fail-closed incremental remediation, not full server-authoritative replay.
**Scope:** Go run API, trusted Pixi player diagnostics, native result notification.
**Issue:** before this change, an HTTP client could POST `{"match":true}` to
`/v1/runs/{id}/verification` and cause the backend to persist
`RunRecord.Verified=true` without a server-run simulation. That check was
*not* a server-attested result, even when an honest browser generated it by
replaying a seed and input trace. A logged-in player can modify HTTP requests.

## Deployed policy for this patch

- A `POST /v1/runs/{id}/verification` that passes the existing
  authentication middleware returns **403**, irrespective of run ID or
  payload. Requests without valid authentication can be rejected earlier by
  that middleware. Neither path allows client attestation. The handler does
  not mutate `Verified` or `ActualScore`, does not reveal run existence
  and is marked `no-store`.
- The browser keeps `window.__dqReplay` for **local diagnostic comparisons**
  and replay timeouts, but does not POST a verdict or treat the local outcome
  as a server-issued attestation.
- Normal submitted run records can remain in the **unverified** board, as
  pre-existing behavior, but the result-card rank is explicitly labelled
  **unverified**. Rank is not a trusted/verified score.
- A native `verified_run_available` notification is emitted **only** when
  the server's run-submission response explicitly says `verified: true`.
  Native independently fetches and checks run/game/session provenance;
  it must not interpret a mere `runId` as verification.
- Existing historical client-issued verification flags are **not
  presented as trusted** through leaderboard or recorded-run read APIs:
  `verified` and `actualScore` are masked in the response copy. The
  original persisted record is left intact as audit evidence. A future
  versioned server-attestation field/migration must distinguish authenticated
  server verdicts from this legacy field; do **not** silently rewrite history.
- Audit/clear or re-attest historical client-issued `Verified=true` records
  separately through a controlled, reviewed migration.

## Versioned replacement protocol (NOT yet implemented)

1. Server accepts a bounded replay candidate only with an immutable game
   package SHA-256, exact versioned IR, seed and complete monotonic,
   allowlisted canonical input history, one authenticated attempt ID,
   and a commit/final-outcome fingerprint.
2. A **server-owned, sandboxed trusted deterministic replay executor**
   independently validates package permission/revocation, reconstructs
   the same 60 Hz semantic trace and outcome, and records a signed,
   versioned attestation bound to immutable package+attempt+auth epoch.
   Never accept `match`, `actualScore`, or `verified` from the browser as
   attestation, including when that browser is trusted to display a game.
3. Finalize idempotently in a server-side transaction with a strict state
   machine: `pending -> verified | rejected`; no client-updatable verdict.
   Return read-only status; emit a positive native result event only after
   a genuinely completed server verification.
4. Reject mismatched game version, deleted/private/revoked packages,
   unsupported semantic action, missing/reordered trace chunk, oversized
   or incomplete evidence, divergence, timeout, replay resource exhaustion,
   stale account/session epoch, and changed outcome. Keep existing valid
   v1.x package readers but fail closed on unsupported attestation formats.
5. Require Go HTTP + browser + Swift golden tests for replay agreement,
   forged positive/negative verdicts (including a guessed run ID), races,
   duplicate commits, win/score manipulation, permission changes, 30/60/120
   visual rates and physical-device replay parity. Raw artifacts belong
   to the relevant issues; simulator screenshots do not prove physical
   iPhone/iPad determinism.

## Release and rollback

- This fail-closed patch intentionally suppresses client-derived
  verification; it may temporarily reduce the number of verified runs,
  which is safer than claiming untrusted scores are proven.
- Rollback requires security review. Re-enabling the old arbitrary
  `match=true` POST handler is **not** an acceptable rollback.
- Require all six exact-head Go validation jobs (tests, PostgreSQL,
  Chromium, WebKit, Playability Kit, cross-browser determinism) before merge.
  No physical hardware result is claimed by this source change.
