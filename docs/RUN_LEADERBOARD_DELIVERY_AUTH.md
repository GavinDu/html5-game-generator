# Run and leaderboard delivery authorization — runtime Epic #202

Parents: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [server replay security #360](https://github.com/GavinDu/html5-game-generator/issues/360), [untrusted ranking #402](https://github.com/GavinDu/html5-game-generator/issues/402), scoped [#417](https://github.com/GavinDu/html5-game-generator/issues/417).

## Security finding and independent implementation

Prior to this slice, `GET /v1/games/{id}/leaderboard` and `POST /v1/games/{id}/runs` only loaded `store.GetGame(id)`. They did **not** recheck the authorization, moderation, quarantine, group membership and publication/visibility restrictions already enforced by package/asset delivery and sensitive `GET /v1/runs/{id}`. A guessed private game ID could expose leaderboard names, scores, run IDs and timestamps; a browser could persist a client-origin provisional score for a restricted or unpublished game.

Both run/leaderboard handlers now call the existing `deliverableGame(w,r)` before reading any board, decoding a submit body or persisting a run. This is the **same shared canDeliver policy** used for game packages; no second consent model. If a game does not exist or the viewer cannot load it, the handlers return an indistinguishable HTTP 404 with `Cache-Control: private, no-store`, never game names or actual privacy state. Every run/board response is viewer-scoped with `Vary: Authorization, X-User-ID` and is non-cacheable to prevent shared-proxy leakage after visibility changes. Approved public game runs and visible public leaderboard queries still work anonymously. A creator may still preview their private own game; other viewers require real authorization under the shared policy.

The Go security regression creates a real approved public game run first, then updates that same game to private, confirms anonymous and outsider GET and POST return 404/no-store with no score-name leakage or durable run-log mutation, while the owner can still view and post. Separate restricted examples cover group-only without membership, quarantined, moderation-rejected and draft game states. The sample game and run are real persisted in-process fixtures; there are no fake score verification flags, no synthetic login authentication in production. Local test-only user identities are enabled solely through the existing `DQ_ALLOW_INSECURE_USER_ID` development flag.

## Senior game-engine architect/security challenger and limits

- Verify that the common `canDeliver` scope, not a new bypass, protects both GET and POST, including 404/no-store and identity variance. Ensure no run ID is allocated and no JSONL append occurs on unauthorized POST. Test owner-preview, public and status/visibility transitions. Repeat all six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit Actions before merge.
- **This is entitlement, not score truth.** Even an authorized viewer's submitted Score/Won remains browser-origin and `rankingScope: provisional`. Independently attested score/win and authoritative-only ranking remain #381/#382/#360/#402. Do not promote historical Verified fields or client SHA as proof.
- CDN invalidation, access revocation of already-downloaded resources, true backend bearer enforcement, user-group governance, real iPhone/iPad hardware, first GPU-present and thermal remain broader Epic gates. This change does not make an insecure development identity header acceptable in production.
- Rollback limited to adding the common gate + cache/identity headers, scoped negative tests and documentation. The senior review is assistant-led adversarial source checking, not independent human or real-device certification.

Close #417 only once the exact current head's six mandatory generator CI jobs pass; Epic #202 stays open.
