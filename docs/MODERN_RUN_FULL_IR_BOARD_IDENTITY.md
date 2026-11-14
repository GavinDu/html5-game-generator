# Modern leaderboard partition uses the full published IR SHA-256

Parent: [Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [trusted input/version #381](https://github.com/GavinDu/html5-game-generator/issues/381), [verified ranking #402](https://github.com/GavinDu/html5-game-generator/issues/402), atomic [#421](https://github.com/GavinDu/html5-game-generator/issues/421).

The legacy leaderboard key is a 12-hex-character prefix of the Game IR SHA-256. Newer canonical run records already persist the *full 64-hex SHA* and bind their canonical seed/input evidence to it. The board previously compared only the short prefix. An otherwise internally consistent replay associated with a distinct 64hex Game IR version but sharing the first 12 hex digits could be ranked under the current game's provisional board. The underlying `runEvidenceStatus` only checked internal consistency of the stored run, not its full digest against current published IR.

`boardRuns` now computes the full hash from **the current published Game IR** exactly once and requires `rec.IRSHA256 == current full digest` for any modern record having a full IR SHA. It applies this before ranking after legacy game-ID/short-key admission. A pre-hash historical run with no `IRSHA256` may still appear on the **provisional** compatibility board with `legacy_unverified` label (no browser Verified promotion). Stored logs, GET diagnostic detail and 12hex API field stay unchanged.

Go regression creates a normal admitted modern run; constructs a second *self-consistent* run with the same 12hex prefix but different 64hex digest and a recomputed canonical input evidence SHA, artificially positioned to win ties; proves it is `unverified` relative to itself but absent from the **current** board. Checks clean modern and true historical legacy remain consecutive `rankingScope:provisional`, client `verified` remains omitted, and foreign stored run is not rewritten.

## Senior game-engine architect challenge

This is defense in depth against a **theoretical 48-bit truncated hash identity collision**, not evidence of a practical collision or a signature. A modern record whose full SHA matches still has browser-submitted Score/Won; it is never independently attested. Separately, truly historical short-key records cannot be safely attributed to a full exact Game IR version and must remain labeled provisional until explicitly migrated or retired. User access scope is independently guarded by merged #417. Exact-head six-job Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit success plus adversarial source review required to close #421. Parent #381/#382/#402 and native Epic #202 remain open. No physical iPhone/iPad GPU/thermal proof or human source review is asserted.

Rollback removes the per-modern-record full hash equality, its regression fixture and this document only.
