# Runtime Epic #202 — verified-only leaderboard read seam (#441)

## Intent
`GET /v1/games/{id}/leaderboard/verified?window=week|all` introduces a separate read-only contract for consumers that need **server-attested** scores. It uses the existing game-delivery authorization policy, current full published IR SHA-256 and score/time sorting metadata, with `Cache-Control: private, no-store` and auth `Vary`. All unexpected, malformed or repeated window query keys fail with 400. The result deliberately exposes `rankingScope:verified_only`, `attestationAvailable:false`, and an **empty** `entries` array.

**Important:** This is not an implementation of independent gameplay verification. No server-owned replay interpreter currently mints a trusted run verdict and no durable server-only verification ledger is deployed. Therefore no historical `Verified:true`, client-supplied score, matching replay hash, plausibility cap or present-day provisional rank is eligible for this new endpoint. It must **never call** `boardRuns` or infer trusted rankings from the legacy `RunRecord.Verified` field. Returning an empty verified board is a truthful result, not a false green.

The existing `/v1/games/{id}/leaderboard` remains explicitly `rankingScope: provisional` for compatibility and may still display browser-submitted score claims. **Security parent #402 is not resolved by adding a safe alternate endpoint**. Product needs to migrate iOS/browser competition UI from provisional ranks to this attested-only read contract and, separately, withhold or clearly label unverified numeric positions. Full server-issued session/seed and complete independent replay #382, durable idempotent verdict ledger #429, and verified-only rank admission #402 remain separate release blockers.

## Reviewer challenge
- This is a schema and authorization boundary only; do not populate it with `Verified:true` from legacy storage or a browser's hash.
- The strict server-defined game/IR version and owner/visibility gate must remain intact across private/group/moderated cases. A no-store empty result cannot reveal a private game to unauthorized viewers.
- Native/HTML5 clients must not treat empty trusted rankings as connectivity failure, claim another player's score was independently verified, or display the provisional board as verified.
- Negative Go tests submit an actual in-memory browser claim that appears on the provisional board; even a manually forged historical `Verified:true` cannot enter the new endpoint. Every method remains read-only.
- Rollback is removal of one additive route/handler, tests and this documentation. No changes to score persistence or iOS app required for this atomic seam.

All six exact-head Go unit/race/static, PostgreSQL, Chromium, WebKit, determinism and Playability Kit CI jobs and assistant senior game-engine-architect adversarial review must pass before closing child #441. App Store 4.7, actual iPhone/iPad GPU-present and physical thermal evidence remain open under Hephaestus Epic #202.
