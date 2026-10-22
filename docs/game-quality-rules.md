# Game-specific quality rules and delivery targets

The scoring package is engine- and runner-independent. Each game selects a declarative rule file; objective evidence comes from the browser adapter and creative scores come from the AI reviewer.

## Delivery target

- Weighted overall score: **at least 80/100**.
- Every one of the 13 dimensions: **at least 70/100**.
- Complete evidence/review coverage; missing values never count as passes.
- All hard playability gates must pass: clean boot, nonblank rendering, and no softlock.
- Every dimension below 70, and every unscored dimension, creates an explicit AI-review request. The reviewer must provide a concrete, prioritized suggestion with evidence for each failing dimension.
- A game is target-ready only when all these conditions pass. The scoring module reports the target; it does not automatically claim that a game is fixed.

## Rule selection

- Shared defaults: `config/game-quality/default.json`.
- `inventory/13ui1k/quality-rules.json`: Bobo's Rainbow — collect-and-restore arcade.
- `inventory/2uowg9/quality-rules.json`: Escape from the Injection — lane-dodging chase.
- `inventory/u46ex5/quality-rules.json`: Ribbon Mends the Sky — action/collect/repair.

Each rule file declares thresholds, metric weights (must total 100), hard gates, a game-specific gameplay contract, audience/fairness constraints, and AI reviewer focus. Keep shared metrics stable for cross-game comparison; tune the evidence and review emphasis per game.

## AI review payload

For every metric in `reviewRequests`, the reviewer should return a score and:
- `rationale`: why this score follows from the observed game;
- `evidence`: specific screenshot, action timeline, browser observation, source location, or reproducible behavior;
- `improvementSuggestions`: one item per below-floor or unscored metric, with `metricId`, `priority`, `problem`, and a concrete `suggestion`.

Do not inflate scores to pass the threshold. Prioritize fixes that raise multiple metrics without breaking the game's intended identity. Re-run the failed test and relevant regression checks after each fix.

## Run

```sh
cd packages/game-quality-score && npm test
cd ../../tools/playtest && npm test
node score-report.mjs playtest-report.json ai-review.json scored-report.json
``

For live playability claims, run the browser worker against a reachable game URL. Reading source or a manifest is not equivalent to playing the game.
