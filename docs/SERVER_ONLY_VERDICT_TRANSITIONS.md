# Runtime Epic #202 — pure server-only replay verdict transitions (#445)

## Purpose and trust boundary
`internal/attestation` is an independent **pure value-state reducer**, not an attestation service or HTTP endpoint. It accepts server-origin `Identity` holding an authorized run ID, game ID, player ID, full canonical published IR digest, canonical v2 trace digest, capability manifest digest, server-selected seed and exact proof version. It validates bounded ASCII identifiers and **64-character lowercase hex** hashes, and creates only an empty `pending` record.

`ApplyServerDecision` accepts a decision only from a *future* independently authenticated server replay worker. It may transition `pending → verified | rejected`. A verified outcome has a bounded integer score, explicit win bit, and 64-hex deterministic state digest; a rejected outcome has a fixed rejection reason with **no** score, win or state digest. Applying **the identical final decision** again is idempotent, but changing seed, IR, trace, game, owner, capability, proof version, state hash, score, win or result status fails closed. No final-to-pending downgrade, client-authored `Verified:true` or legacy `ActualScore` is accepted by this module.

## Senior game-engine-architect challenger limits
- **This pure reducer authenticates nobody.** A malicious caller able to invoke it with spoofed `Identity` and `Decision` could still produce a `Verified` value. Only a future inaccessible-to-client issuer/replay-worker pipeline may feed it. Its exported types and helper must NOT be made into a new public HTTP trust endpoint.
- **No durable compare-and-swap:** Concurrent workers and process restarts are not safe until #429 implements a transactionally persisted ledger keyed by an issued attempt, with idempotent replay job IDs, expiry/credential revocation and double-reward prevention.
- **No replay execution:** The existing Go `semanticreplay` route is diagnostic with **client-selected seed and trace**. This helper does not alter that route, decide which games are replayable or upgrade their score. Full independent capability-scoped replay and provenance remain #382/#428/#427.
- **No leaderboard:** Existing provisional browser ranks are still client claims. Verified-only additive read seam #441, actual UI migration and trusted-rank admission #402 are separate. Do not copy this policy's statuses into user-facing APIs until server attestation exists.
- **No mobile compositor claim:** Pure Go unit/race/browser CI cannot observe a physical iPhone/iPad first GPU present, 50-cycle teardown or App Store approval (#276/#279/#282).

Rollback removes this new package, its tests and these notes. Close only #445 after a skeptical senior engine/security source review and all six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit jobs pass.
