# Provisional run provenance — API compatibility slice

Links: [Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [ranking security #402](https://github.com/GavinDu/html5-game-generator/issues/402), [scoped API #403](https://github.com/GavinDu/html5-game-generator/issues/403), [server attestor #382](https://github.com/GavinDu/html5-game-generator/issues/382).

## Current trust invariant

The backend has **no independent semantic replay/score attestor yet**. Existing v1 run submissions include browser-supplied `score` and `won`; the server computes and stores an input evidence fingerprint, but it cannot prove the claim. Historical stored records may have client-origin `Verified:true` and `ActualScore` fields and must never become trusted.

This additive response-only slice labels present-day ranking clearly:

- `rankingScope: "provisional"` for each v1 POST submission, leaderboard entry, and authorized GET run. Existing numeric `rank` and board sorting remain backward compatible while native/web UX migration is designed.
- `evidenceStatus: "unverified"` when a modern record has a consistent server-owned evidence fingerprint, `legacy_unverified` when historical records have no modern evidence, and `invalid_evidence` for malformed modern stored evidence.
- These fields are **recomputed by the server**, never accepted from the browser or persisted as a purported result verdict. Legacy client-writable `Verified`/ `ActualScore` are ignored and remain masked in public responses. The sensitive GET trace keeps existing authorization and no-store constraints.

## Challenger conditions and exclusions

Explicit **provisional** metadata is a trust transparency improvement, **not** authoritative rank isolation. In current v1 compatibility mode, browser claims can still affect the provisional board. [#402](https://github.com/GavinDu/html5-game-generator/issues/402) remains open until unverified claims are excluded from any *authoritative* leaderboard, the browser/iOS UI labels provisional results, historical records are migrated and [#382](https://github.com/GavinDu/html5-game-generator/issues/382) implements server-computed score/win. Do not mint a pending verifier state while no verifier worker exists. Do not equate a matching SHA with an honest game session.

Negative fixtures must cover historical `Verified:true`, altered stored seed/digest, GET authorized replay, board and post responses. All six current-head Go/race, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit jobs are mandatory. No simulator or physical iPhone/iPad claims arise from Go tests.

Rollback removes additive public response fields and the ephemeral helper without rewriting legacy rows.
