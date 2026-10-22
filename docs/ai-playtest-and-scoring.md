# AI Playtest Kit and Game Quality Scoring

Status: design contract + first scoring implementation. The existing Playwright worker remains the source of objective browser evidence.

## Goal

Before a generated game is delivered, automatically:

1. load it in a real browser and detect load/runtime/render failures;
2. mimic a player using game-aware actions, not just arbitrary input;
3. verify input, state progress, rewards, end states, restart paths, and softlocks;
4. produce screenshots, traces, runtime evidence, and a machine-readable report;
5. ask the coding/generation agent to diagnose failures and propose a bounded fix;
6. rerun the same test plus regression tests against the candidate;
7. score technical playability separately from creative quality;
8. deliver only when hard gates pass; never turn unmeasured checks into passes.

The current `tools/playtest/probe.mjs` is a generic baseline bot and runtime probe. It is useful for broad smoke coverage but cannot prove that a game is fun or that its intended win condition is reachable. A future game-aware agent should consume the Game IR, control schema, goal/lose conditions, scene census, and browser observations to choose actions.

## Test stages

### Gate A — package and boot

- Package/API request succeeds; all referenced assets resolve.
- No page errors, unhandled rejections, runtime error telemetry, or failed required asset loads.
- Runtime reaches its first simulation frame within the configured timeout.
- Stage is nonblank and contains visible game entities.
- No unrecognized IR capability silently appears to execute.

### Gate B — interaction and progression

- Controls are visible or discoverable and input is accepted.
- Every declared input binding is exercised at least once.
- State changes after meaningful actions; no stuck/frozen simulation under input.
- The bot observes at least one intended reward/progression signal when the game contract declares one.
- Win and lose conditions are attempted where declared; inability of a generic bot to reach them is inconclusive, not automatically a game failure.
- Restart/replay restores a playable state.
- Run deterministic seeds and bounded fuzz sequences where supported.

### Gate C — resource and performance budgets

- Cold-start-to-playable latency; first input latency; first meaningful feedback latency.
- Frame-time distribution (median, p95, p99), long-frame count, frame stalls.
- Memory/texture/audio footprint and entity count where observable.
- Console errors, network failures, asset sizes, package size.
- Mobile viewport, touch input, resize/orientation, and WebKit/WKWebView coverage.

### Gate D — AI review and repair loop

The agent receives: IR/Game Package, design intent, control bindings, goal conditions, failed check IDs, console/network logs, screenshots, action timeline, state census deltas, and browser trace.

The agent must return a structured diagnosis:
- classify defect as runtime/platform, package/assets, controls, game logic, difficulty/softlock, visual/animation, or design mismatch;
- cite concrete evidence and affected IR paths/files;
- propose the smallest fix;
- state a regression test that should fail before and pass after the fix.

Run at most three repair iterations by default. Each iteration must rerun the failed check and the regression suite. Do not auto-apply changes that weaken checks, alter the intended goal to make it easier to pass, remove evidence, or suppress errors. If the model cannot identify a safe fix, report the game as blocked for human review. Current repair endpoints are shadow-mode and deterministic; LLM-driven repair is not yet wired, so this document does not claim the end-to-end AI repair loop is already active.

## Scoring model

A score is a quality signal, not a substitute for hard delivery gates. The weighted dimensions total 100:

| Dimension | Weight | Primary evidence |
|---|---:|---|
| Minimum playability | 20 | Browser boot, visible scene, live state under input, no softlock |
| Runtime reliability | 12 | Exceptions, asset failures, unsupported capabilities |
| Input responsiveness | 8 | Time to input and first meaningful reward |
| Performance | 8 | Cold start, frame times, memory and package budgets |
| Visual beauty and cohesion | 8 | Screenshot/scene review: composition, contrast, color, polish |
| Character design | 7 | Silhouette, personality, consistency, readability |
| Animation and game feel | 7 | Motion timing, transitions, impact, feedback |
| Plot and emotional arc | 7 | Premise, progression, stakes, payoff |
| Strategy and meaningful choices | 8 | Agency, trade-offs, skill expression, viable alternatives |
| Level and progression design | 5 | Pacing, spatial clarity, variety, difficulty curve |
| Audio and feedback | 4 | SFX/music usefulness, synchronization, mix, mute |
| Originality and coherence | 4 | Distinctiveness and consistency between theme and mechanics |
| Usability and accessibility | 2 | Legibility, control clarity, input alternatives, difficulty fairness |

The scoring library is `tools/playtest/quality-score.mjs`. It combines objective measurements with optional AI review scores. AI review dimensions require both a rationale and evidence; unsupported scores are ignored. Objective scores cannot be overridden by the AI. If some dimensions are unassessed, `overallScore` stays null and `partialScore` is labeled with its coverage. This avoids presenting a partial score as a complete evaluation.

## Delivery policy

- Hard gates: `playtest.boot_clean`, `playtest.render_nonblank`, and `simulation.no_softlock` must all pass.
- A skipped/inconclusive hard gate means delivery is not yet verified.
- A failed hard gate blocks delivery regardless of the creative score.
- Suggested quality policy once the AI rubric is integrated: complete score >= 70, no individual creative dimension below 40, and all hard gates pass.
- Keep raw measurements, AI rationale, model/version, screenshot references, browser/version, game package hash, and repair attempt history in the report for reproducibility.

## AI review rubric contract

For every AI-scored metric, return:
```json
{
  "id": "strategy",
  "score": 0,
  "rationale": "Specific assessment, not generic praise.",
  "evidence": "Observed choice A/B and the resulting state change in screenshot/trace/IR."
}
```

Use a 0–100 scale. Scores should be anchored: 90–100 exceptional, 75–89 strong, 60–74 adequate, 40–59 weak, 0–39 broken/absent. Evaluate against the declared genre, audience, intended session length, and creator intent; do not penalize a deliberately simple child-friendly game for not having complex strategy. When evidence is unavailable, omit the score instead of guessing.

## Run tests

From `tools/playtest`:

```sh
npm test
```

The current worker can still be run against a live server using the documented `go run ./cmd/playtest -base ...` command. Browser tests require Playwright and its browser binaries to be installed.
