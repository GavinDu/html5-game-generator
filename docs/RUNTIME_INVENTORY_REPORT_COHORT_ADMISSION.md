# Fail-closed inventory replay comparison admission — Epic #202 / child #488

The existing Playwright inventory semantic replay test already enumerates `inventory/*/manifest.json`, runs the shipped Pixi runtime, captures real fixed-tick state at a pinned seed and uploads browser screenshots. The matching cross-browser comparison previously trusted `gameIDs`, `seed` and `simStep` values asserted by the two report files themselves. Two identically incomplete or internally inconsistent browser reports could pass without proving that every **checked-in** shipped game and its declared seed/tick cohort were actually compared.

## Atomic independent fix

`compareInventorySemanticReplay` now determines the complete sorted list of game IDs **from the current repository's checked-in inventory manifests**, checks that game IDs are nonempty canonical slugs and unique, requires both Chromium and WebKit reports to list exactly that source inventory, validates safe bounded report-level seed and exact virtual-tick metadata, requires the two reports to agree on those headers, and insists each game/viewport entry's declared seed, tick and observed state tick match the enclosing report. It retains strict same-game/state equality across engines and rejects any missing, unexpected or duplicated game/profile. No state normalization tolerances, screenshot substitutes or browser score forgery were introduced.

The unit test suite includes two browser reports agreeing on a **missing game** (must reject), duplicated or replaced game ID (must reject), both report headers agreeing on a different seed/tick while their entries disagree (must reject), one report’s changed header, and a single game's mismatched seed/tick. Existing strict drift, missing and duplicate profile tests remain required. The test is already included in the mandatory Node suite in each real Chromium and WebKit job.

## Senior game-engine architect challenger / nonclaims

This proves only **local CI report integrity and deterministic manifest coverage**, not remote package authenticity, original immutable Game IR SHA, the authority that assigned the seed, actual input-trace completeness, event-by-event reducer agreement, slow/fast pacing, browser-to-physical-device parity, nor independent server-owned run verification/ranking. Reports and checked-in inventory could still be modified together in a bad repository commit; trust ultimately requires immutable signed releases and source review. The parent all-inventory fingerprint/slow-fast corpus [#389](https://github.com/GavinDu/html5-game-generator/issues/389) remains OPEN until at least three synthetic genres, fixed slow/fast render pacing, input/IR digests and terminal semantic parity are covered. Server replay #428/#382/#402 and native actual iPhone/iPad compositor/thermal #276/#279 remain separate.

## Validation and rollback

Require all six **exact-head** Go/race/Postgres/real Chromium/real WebKit/Playability Kit/cross-browser-determinism jobs SUCCESS before squash merge and closing **only child #488**. Fix any Node regressions at the source rather than bypassing the smoke test. Rollback removes the pure comparator bounds, extra negative unit tests and this doc, with no production runtime behavior change. No paid Replit models or workflows.
