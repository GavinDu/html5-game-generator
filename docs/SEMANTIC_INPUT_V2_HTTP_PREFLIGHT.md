# Published Game IR-bound semantic input v2 diagnostic preflight

Parent [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), full v2 wire producer/admission [#381](https://github.com/GavinDu/html5-game-generator/issues/381), independent server-owned attestor [#382](https://github.com/GavinDu/html5-game-generator/issues/382), scoped [#423](https://github.com/GavinDu/html5-game-generator/issues/423).

## New additive route, not a run submission

`POST /v1/games/{id}/semantic-input-v2/validate?irSha256={lowercase64hex}&seed={canonicalUInt32}`

Body is exactly the canonical `semantic-input-trace-v2` JSON object with `schema`, `durationTicks` and `events`; `Content-Type: application/json` (UTF-8 charset accepted). The endpoint never accepts `Score`, `Won` or a browser verification verdict, mints no run ID, appends no JSONL, and does not update a leaderboard or game state.

The handler uses `deliverableGame` (the shared package/asset entitlement and moderation authorization) **before** looking at IR identity or body. It revalidates the current persisted published IR and computes the full 64-hex SHA **server-side**. Exactly one `irSha256` query parameter must match that value; exactly one `seed` must be decimal canonical uint32; duplicated/aliased/unknown params and stale hashes fail without touching runs. A bounded 1MiB `http.MaxBytesReader` protects allocation, then the pre-shipped `replaytrace.DecodeStrictV2` refuses duplicate JSON keys, malformed Unicode surrogate escapes, unknown/nullable/case-aliased fields, out-of-order tick/ordinal, undeclared actions, unbalanced press/release/cancel, invalid pointer fixed-coordinate and event budget overflow.

The canonical evidence hash is derived using the current trusted published Game IR, gameID, server-computed full SHA, client-declared bounded seed, and accepted trace. HTTP 200 includes `schema:semantic-input-admission-v2`, `gameId`, `irSha256`, `seed`, `traceSha256`, `durationTicks`, `eventCount` and **`evidenceStatus:unverified`, `rankingScope:none`, `attested:false`**. All responses are `Cache-Control:private, no-store` and `Vary: Authorization, X-User-ID`; errors never echo submitted input or protected Game IR.

Go regression checks valid real published Game IR idle and declared down/up input; duplicate/case-aliased/unknown/null fields, invalid schema, stale full hash, invalid/missing/duplicate seed, wrong MIME, oversize, non-existent game, private owner vs outsider and unchanged in-memory run count, with a stable identical-input canonical digest.

## Senior game-engine architect/security challenge

This is an **admission-only diagnostic**. The seed is still client-supplied and no trusted server interpreter has replayed it. A SHA-256 checksum detects changed submitted evidence, but is not a signature, publisher-authenticated record, anti-cheat proof or verified win/score. Historical v1 run POST stays provisional; client POST /verification stays forbidden. Live JS producer and durable immutable authenticated v2 run storage are still #381, independent deterministic gameplay interpreter and trusted attestor #382/#360, verified-only board #402. A public guest may validate public input, but private/group/blocked game identity must be hidden. Do not replace source-authorized player access with CORS as an authorization mechanism.

Close only scoped #423 after all **six exact-head** Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit jobs success plus assistant-led senior game-engine architect code review. This does not prove native iPhone/iPad 50-cycle memory/thermal, first GPU compositor frame or independent human reviewer signoff. Parent Epic #202 remains open.
