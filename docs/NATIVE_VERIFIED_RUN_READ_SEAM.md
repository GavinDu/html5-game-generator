# Native verified-result read seam — Hephaestus Runtime Epic #202, atomic #447

## Why this route exists

The shipped iOS `BackendAPI.fetchVerifiedRunResult` requests `GET /v1/runs/{id}/result` after a narrow, validated WebKit run-ID notification. Previously the generator offered no such route: `GET /v1/runs/{id}` is sensitive *diagnostic* replay evidence and strips historical client-supplied `Verified` and `ActualScore`. The independent Go server replay verifier (#382) and server-only verdict ledger (#429) do not yet exist.

This additive route is **read-only and fail-closed** until that work is deployed. It bounds the opaque run ID to 128 safe ASCII bytes and rejects queries. Unknown, private to an unauthorized user, deleted, blocked or moderated runs return indistinguishable 404. A currently deliverable known run returns **503 attestation unavailable**, never a fake 200 verified result, even if its historical `Verified` flag or replay hash claims success. The endpoint never exposes a run score, result, player identity, trace, legacy verdict or rank and never changes stored runs. All responses are `Cache-Control: private, no-store` with `Vary: Authorization, X-User-ID`. The authorized unavailable response includes `Retry-After: 3600`.

## Contract and adversarial senior game engine architect challenge

- **Trust root absent:** This route cannot produce a native `VerifiedRunResult` today, by design. The backend must implement independently executed canonical seed/inputs/tick semantics and a server-only idempotent verified ledger first. A browser-sent hash, `verified:true`, persisted `RunRecord.Verified` or a lucky score never qualifies.
- **Safe eventual positive schema:** A future implementation should version `runId`, `gameId`, immutable full `irSha256`, `status`, `attested`, `rankingScope`, `evidenceStatus`, `result`, and optional trusted rank; issuer and viewer authorization must bind to exact published game/IR and verdict. The native side (atomic #397) now requires `attested:true`, `rankingScope:verified_only`, `evidenceStatus:verified` plus actual outcome to show trusted UI.
- **Privacy:** As long as existing diagnostic `GET /v1/runs/{id}` permits a viewer to fetch published-game evidence, a 503 for a present *deliverable* public run does not introduce a stronger existence disclosure. Private/revoked records stay 404. Further per-run privacy and attestor policy must be reviewed before enabling a 200 verdict.
- **Determinism:** No new semantic engine or scoring algorithm. No game-specific branches, dynamic code or privileged JS/native API.
- **Tests:** Go negative matrix covers forged historical verified score, private/other player/owner access, orphan IDs, revocation, invalid IDs/query, response no-cache and zero ledger mutation.
- **Rollback:** Remove this handler, route, tests and doc; no persistent migration. A 404 for every request was the previous behavior. No actual iPhone/iPad GPU, real device thermal or 4.7 audit proof follows from Go CI.

Do not merge this atomic contract until all six exact-head required GitHub Actions succeed, including Playability and browser determinism, and an assistant-led adversarial senior architecture review is recorded. Close only #447, never #382/#429/#402 or Hephaestus Epic #202, from this safe negative contract alone.
