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
- `inventory/p3ngui/quality-rules.json`: Penguin's Heartlight Rescue — gentle collect-and-restore family arcade.

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

## Source fidelity and media provenance (2026-11-09)

A browser screenshot can establish that a sprite rendered, but it does not establish that the original drawing's pixels were used. The stub `media.SpritePNG(label)` creates generated artwork. Mark those emitted sprite assets `source: generated`, not `kid-drawing`; reserve `kid-drawing` for assets that actually preserve the creator's drawing with recorded rights and source provenance. Do not inflate character-design or source-fidelity scores based on a generated sprite's title alone. Review actual screenshots against the approved questionnaire and source reference.

## Screenshot-led creative and stability acceptance (proposal, 2026-11-09)

These review rules use the real captured iPhone/iPad evidence from Hephaestus
CI run [42677300302](https://github.com/GavinDu/Hephaestus/actions/runs/42677300302)
and the Chromium/WebKit game/feed captures from generator run
[42676568183](https://github.com/GavinDu/html5-game-generator/actions/runs/42676568183).
They **do not certify footage as reviewed** and must not be counted as
implemented automated gates until the corresponding checks actually exist.

### Game-specific creative checks

| Game | Evidence to demand before choosing a social poster or clip |
| --- | --- |
| Bobo's Rainbow | Actual touch-driven catches (not a fabricated score) have restored multiple recognizable rainbow bands; the original drawn character is not cut off. The observed 4/12-heart candidate has an honest score but a small upper-left arc and a large empty sky, so it is a *review candidate*, not automatic poster approval. |
| Penguin's Heartlight Rescue | The seven-second introduction really clears; catch progress, all three source-inspired scarf variants, intact penguin silhouette, visible aurora progression, real 12-heart ending and restart are separately documented. A blocked gameplay view under an opaque intro card is not a good feed cover. |
| Ribbon Mends the Sky | A screenshot must show an actual repaired sky hole and visible ribbon/gameplay response, not merely an attractive untouched introduction. Verify the dancer and ribbon are not clipped on phone/tablet. |
| Escape from the Injection | Capture the real moving lane hazard, player agency and outcome; a still empty lane or a controlled collision fixture alone cannot establish the natural long-run balancing or 15–20-minute goal. |

Creative review is relative to the supplied source drawing, the game-specific
questionnaire, accessibility and intended audience. Preserve distinguishable
character silhouettes and color choices rather than substituting stock art.
If an asset was generated rather than directly preserving creator pixels,
label its provenance accordingly. Reject a screenshot that misrepresents
what was actually playable, including stale IR/version bindings.

### Feed and device presentation checks

- The Instagram/Reels and YouTube Shorts references inform hierarchy:
  one dominant playable stage, readable title/creator, right-side
  social controls, and a separate high-priority **Play** action. Do not
  import their branding, fake social counts, or unapproved preview media.
- Validate screenshots and **real feed navigation** independently on iPhone
  portrait/landscape and iPad portrait/landscape. Record the input method
  actually tested; wheel or keyboard navigation is not proof of touch swiping.
- Maintain 44×44 CSS-pixel / 44-point actionable targets, safe areas,
  accessible contrast, Dynamic Type, no unintended horizontal overflow,
  and a game card that changes after a swipe without entering gameplay.
- Raw, native iPad landscape screenshots can carry TIFF/EXIF orientation
  metadata; calculate displayed dimensions and visually inspect original
  images. Never silently rotate an attachment and claim it was raw.
- Score three separate outcomes: **functional browsing**, **visual validity**
  and **reviewed gameplay media**. A passing swipe/UI score with an explicit
  preview placeholder is not an approved poster or clip.

### Fail-closed stability rules

- No clip/poster is marked ready unless its source is a real gameplay capture,
  bound to the exact Game IR hash/version, visually reviewed, and deliverable.
  Pending/failed/stale/incomplete media must not leak a playable clip URL.
- Missing or timed-out game-aware playtest workers block *new publication*,
  while preserving an actionable creator-facing error; a green fixture CI
  run is not a generation publish gate. See #98.
- Any missing required GitHub Actions job is **unverified**, not successful.
  Confirm exact PR-head checks and full post-merge `main` jobs before calling
  either repository release-ready.
- Keep screenshot provenance: source run, original bytes, viewport,
  browser/simulator, orientation, input trace, progress and scoring state.
  When a screenshot cannot be captured, fail visibly with diagnostic evidence.

**Rollout:** first make these rules measurable in the browser/XCTest evaluators,
then enforce them without weakening current game mechanics or quality gates.
Photo-based artwork and generated artwork must retain explicit provenance.
