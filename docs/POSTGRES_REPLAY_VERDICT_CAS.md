# Private PostgreSQL replay-verdict compare-and-swap — #453

Parent: [Hephaestus runtime Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [server verdict ledger #429](https://github.com/GavinDu/html5-game-generator/issues/429), [independent replay attestor #382](https://github.com/GavinDu/html5-game-generator/issues/382).

## Architecture and trust boundary

`internal/attestation/PostgresVerdictStore` is a **private, unwired** Go SQL adapter. It does not accept HTTP, connect to a user-readable leaderboard, mutate historical `RunRecord.Verified` / `ActualScore`, or claim a replay verdict is independently computed. Existing public leaderboard/ranking remains provisional and unverified where applicable.

An immutable `ServerIssuedAttempt` binds full `runId`, `gameId`, `playerId`, full 64-hex `IRSHA256`, `TraceSHA256`, `CapabilitySHA256`, server-controlled seed and `ProofVersion`, plus 64-hex authorization-scope digest and canonical microsecond-precision UTC issue time. No bearer/age/secret or raw input trace is stored. `CreatePending` is idempotent for the **exact** issued identity only. A different issue time/scope/digest/player under the same run ID conflicts.

`CommitServerDecision` requires the full originally issued attempt, validated global replay-job ID and pure `ApplyServerDecision` validation. A PostgreSQL `SELECT ... FOR UPDATE` serializes conflicting workers; a pending-only conditional `UPDATE` fences stale attempts; unique `final_job_id` prevents the same replay job from finalizing two different run IDs even across processes. Same job + exact terminal decision is idempotent; different job/score/win/fingerprint/rejection is a hard conflict. The **PostgreSQL transaction commit**, not a Go mutex, is the durable linearization point. Unique and shape constraints, plus a database trigger, reject identity mutation and terminal downgrade even through another SQL connection. Failed transactions roll back without yielding a partially verified record.

Migration only creates a separate `runtime_attestation_verdict_v1` table and a narrow immutability trigger. It does not rewrite/reset legacy provisional results. Service rollout must supply a separately provisioned PostgreSQL connection; this change does not enable runtime API or publisher entitlements.

## Independent tests

Pure Go tests reject invalid auth digests, timestamps, proof versions and full IR hash. Real PostgreSQL tests (run through the existing `DQ_SOCIAL_DATABASE_URL` CI service) exercise two independent database connections racing 18 mixed verified/rejected workers, exact same-job terminal idempotence, cross-run job ID uniqueness, changed auth scope/issue time/seed, repeated migration/restart, rollback before commit, and direct SQL attempts to rewrite immutable identity or terminal verdict. The CI workflow extends `socialgraph-postgres` to run `./internal/attestation` with the race detector.

**Acceptance gate:** all six exact-head Go validation, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit jobs succeed; senior game-engine-architect adversarial review includes SQL isolation, replay-job collision, transaction abort, and forged issuer boundaries. No merge/closure from an old green run or a partial matrix.

## Explicit limitations that keep #429 / #382 / Epic #202 open

- Methods are exported within an internal Go package but **caller authorization is not implemented**. An authorized, server-controlled attempt issuer must bind the actual account, package version, moderation, expiration, revocation and immutable seed before a ledger call. Treat this type as a storage contract, not an authentication credential.
- There is **no independently trusted Go full-game replay** here; a caller that constructs `Verified` is not proof of actual gameplay. Keep the package unwired to all rank/reward/score endpoints. Ledger rows created in tests are not human gameplay.
- Worker idempotency applies to verdict writes only. Transactional outbox for leaderboard rewards, independent execution claims, issuer revocation, cross-service migration and real WebKit/Pixi trace conformance remain separate.
- Row locks plus database uniqueness are adequate under PostgreSQL READ COMMITTED for a single-row terminal CAS; no optimistic snapshot assumes process-local visibility. SQL schema owner permissions must be restricted before production exposure.
- No physical iPhone/iPad GPU-first-pixel or 50-cycle memory/thermal evidence, Apple App Store acceptance or developer account endorsement is implied.

Rollback: leave the standalone table data intact (never auto-drop historical verdicts), stop invoking the private adapter, and revert only package source/tests and its dedicated PostgreSQL CI invocation.