# Private Go game_start diagnostic reducer — Epic #202 / #471

## Scope

This unexported pure Go helper accepts only strict, canonical Game IR validated by `supportedPublishedIRProfile` in `internal/attestation/game_ir_profile.go`. It executes one diagnostic `game_start` rule sequence. The preinstalled Pixi source initializes `state.score = 0` and walks authored `ir.rules` and `rule.actions` in order; the only allowed action is `add_score` with an explicit bounded signed integer, equivalent to Pixi's `Math.trunc(action.value ?? 1)` for this subset. The produced ledger contains the global ordinal, rule ID, action index, bounded delta and resulting score after each step.

Validation and execution are **both fail-closed**: invalid Game IR is distinguished from unsupported but valid semantics; no score/steps appear on either denial. The preflight permits at most eight start rules and eight actions per rule, no inputs, physics, assets, entities, timers, variables, conditions, terminal actions, scene mutations, shells or other events. Repeated independently decoded Game IR bytes reproduce an identical ordered ledger. The implementation deliberately has no user/seed, network, runtime DOM, random state, saved run, verdict, result endpoint or leaderboard writes. It does not mint authority over caller-provided in-memory Go objects.

## Strict evidence limit

This is an *implementation slice*, **not** server replay attestation. It does **not** exercise the live Pixi runtime, prove the entityless no-input Game IR boots on WebKit or Chromium, or process full v2 event traces. The separate genuine browser-versus-Go fixture and replay acceptance issue [#469](https://github.com/GavinDu/html5-game-generator/issues/469) stays open. Fixed-step multi-event replay [#428](https://github.com/GavinDu/html5-game-generator/issues/428), authentication/publication issuer #457, full independently verified gameplay [#382](https://github.com/GavinDu/html5-game-generator/issues/382), and ranking [#402](https://github.com/GavinDu/html5-game-generator/issues/402) all remain open; iOS physical device gates remain open as well. If the minimal IR fails to boot in real Pixi, reject this profile before any attestation rather than allowing Go to invent valid outcomes.

The tests cover authored order (including intermediate negative score), repeated strict JSON decode, invalid vs unsupported packages, per-rule and total work budget, unsupported physics/input/target/conditions/coercion and absence of partial accepted state. No CI shortcuts or paid Replit. Merge only on all six exact-head GitHub Actions jobs and a skeptical source review with the above limitations.

Rollback: remove the private reducer, its tests and this document; no runtime entry point or persisted data is affected.
