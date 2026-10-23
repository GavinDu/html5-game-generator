# Game Quality Score

Standalone JavaScript scoring package with no runtime dependencies.

- Objective browser checks and AI creative review are separate sources.
- Shared rubric supports per-game JSON rule files.
- Default delivery target: overall score >= 80, every metric >= 70, complete evidence, and all hard playability gates pass.
- Low or missing dimensions are returned as `reviewRequests`; below-floor dimensions require evidence-backed `improvementSuggestions`.
- The package never launches a browser, calls an LLM, or modifies game source.

See [game-specific rule contract](../../docs/game-quality-rules.md).
