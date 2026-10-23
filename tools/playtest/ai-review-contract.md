# AI game reviewer contract

The AI reviewer receives four inputs: the complete game package (manifest, Game IR and game JavaScript), the selected `quality-rules.json`, the browser playtest report (checks, action timeline, runtime errors and screenshots/artifact paths), and the score engine's `reviewRequests`.

**Blind multi-pass protocol (scorecard fusion):** judging runs as (at least) two independent passes over the evidence pack's judge view, which contains observations only — no check statuses, no objective scores, no other pass's output. Each pass returns the JSON shape below plus a top-level `judge: {model, version}`. Passes are returned together as `{ "<gameId>": { judge, passes: [passA, passB, ...] } }`; `score-report.mjs` merges them (spread > 15 on a dimension = contested; a third pass's median stands) and stamps provenance into the scored report. A pass must never see the delivery target outcome it is feeding.

For every request:

1. Inspect the actual game code and observed browser evidence; do not score from the title/intent alone.
2. For a subjective metric, return a score from 0 to 100, rationale, and concrete evidence (specific screenshot, observed action/result, or source location).
3. For any metric below the configured floor (default 70), return a concrete improvement suggestion with priority, problem, and an implementable change.
4. For an objective metric with missing browser evidence, do not invent a score. Explain the missing test/instrumentation and propose the test needed to measure it.
5. Never inflate a score to reach the overall target. Objective metrics cannot be overridden by the AI.
6. Focus on fixes that preserve the game's core identity and raise multiple dimensions without introducing regressions.

The AI-review JSON shape is:

```json
{
  "sample-rainbow-bear": {
    "metrics": [
      {
        "id": "visual_design",
        "score": 68,
        "rationale": "The HUD overlaps the instruction at the portrait viewport.",
        "evidence": "Screenshot: /artifacts/13ui1k.png; source: inventory/13ui1k/game.js:42",
        "priority": "high"
      }
    ],
    "improvementSuggestions": [
      {
        "metricId": "visual_design",
        "priority": "high",
        "problem": "The HUD and instruction text compete for the same vertical space.",
        "suggestion": "Move the instruction below the score row and reserve a safe-area margin."
      }
    ]
  }
}
```

The scorer reports `reviewRequests` for every missing or below-floor dimension and `missingSuggestions` if the reviewer failed to supply a suggestion. A game is target-ready only when overall score is at least 80, every metric is at least 70, coverage is complete, and every hard playability gate passes.
