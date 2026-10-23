# Game Quality Score

Standalone JavaScript scoring package with no runtime dependencies.

- Schema 1.2: objective dimensions are graded from measured values (verdict frame statistics, input/reward timings, error counts) passed as the optional `measurements` argument; the output reports `subscores.measured` and `subscores.creative` separately, caps the measured subscore at 40 when a hard gate fails, and enforces `minimumMeasuredScore` (default 60) in the delivery target.
- Objective browser checks and AI creative review are separate sources.
- Shared rubric supports per-game JSON rule files.
- Default delivery target: overall score >= 80, every metric >= 70, complete evidence, and all hard playability gates pass.
- Low or missing dimensions are returned as `reviewRequests`; below-floor dimensions require evidence-backed `improvementSuggestions`.
- The package never launches a browser, calls an LLM, or modifies game source.

See [game-specific rule contract](../../docs/game-quality-rules.md).
