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

The scoring library is the standalone package `packages/game-quality-score/`. It has no browser or LLM dependencies and can be used by any game generator, CI pipeline, or evaluation service. It combines objective measurements with optional AI review scores. AI review dimensions require both a rationale and evidence; unsupported scores are ignored. Objective scores cannot be overridden by the AI. If some dimensions are unassessed, `overallScore` stays null and `partialScore` is labeled with its coverage. This avoids presenting a partial score as a complete evaluation.

### Graded scoring and subscores (schema 1.2, scorecard fusion)

Since the fusion, the objective dimensions are **graded** from measured values rather than binary pass/fail: playability starts at the hard gates and deducts for an unobserved win, failed shell conformance, failed determinism/replay, or a missing first reward; reliability grades runtime errors and warnings; input responsiveness scores measured first-input/first-reward timings against the IR's declared targets plus the dead-input rate; performance blends cold start with frame-histogram jank rate and p95 frame time from the runtime verdict. The output always reports two subscores alongside the blend — `subscores.measured` (the four objective dimensions) and `subscores.creative` (the nine judged ones) — because they answer different questions and drive different actions. A failed hard gate caps the measured subscore at 40, and the delivery target additionally requires the measured subscore to reach `minimumMeasuredScore` (default 60).

### Judging protocol

Judges score from an **evidence pack** (`score-report.mjs --pack`): screenshots plus a judge view of observations (scene census, HUD, event tail, the IR experience block, creator intent) that deliberately contains **no check statuses and no scores**. Two **blind** passes score independently; `score-report.mjs` merges them — a spread over 15 on a dimension marks it `contested` and a third pass's median stands. Reviews carry judge provenance (`judge: {model, version}` + the pack hash), which is stamped into the scored output. The three shipped games are frozen as a calibration set under `config/game-quality/calibration/`; a judge model or prompt change must re-score them, and a mean drift over 7 on any dimension blocks adoption until reviewed. Each game's `quality-rules.json` contract is **generated from its IR** (`score-report.mjs --write-contracts`); the validator's `quality.contract_matches_ir` check fails when the contract drifts from the game or its prose claims mechanics the IR contradicts.

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

Run standalone scoring tests (no browser binaries required): `cd packages/game-quality-score && npm test`.

Run worker integration tests: `cd tools/playtest && npm test`.

The current worker can still be run against a live server using the documented `go run ./cmd/playtest -base ...` command. Browser tests require Playwright and its browser binaries to be installed.

## Phase E — incidents, tuner, and the suggestions loop

- **Experience incidents**: `GET /v1/telemetry/incidents` also returns `experience` entries derived from telemetry v2 aggregates (jank rate, dead-input rate, win-rate extremes, slow first reward), each with its measured value, threshold, and session count. Rate claims require a 20-session floor; below it, silence.
- **Difficulty tuner**: `POST /v1/games/{id}/tuner/propose` reads the game's measured win rate and proposes ONE pacing knob (ramp speed or health) in shadow mode when the rate leaves the healthy band; `.../tuner/proposals/{pid}/apply` applies it through full IR revalidation, `/discard` drops it. Insufficient data is reported as insufficient data.
- **Suggestions loop**: the AI reviewer covers creative dimensions; objective dimensions get deterministic measured-evidence suggestions in `score-report.mjs`. After merging, `missingSuggestions` must be empty, and `--suggestions-md` renders the report a human reads. Suggestions feed fixes; fixes are re-playtested and re-scored against the frozen calibration cards.
- **SFX as data**: games may ship event sounds as `.b64` WAV files listed under the manifest's `sfx` array; the inventory loader publishes them as artifacts so IR audio assets and `play_sound` rules resolve.
