# Feed ranking: baseline, signals, and rollback

## Current policy

The candidate quality policy remains behind `DQ_FEED_RANKING_ENABLED`; set it to `false` for immediate newest-first rollback. The current process-local event index is the only signal source; missing impressions are not fabricated. In candidate mode, the score adds smoothed play-starts-per-impression, duration-normalized completion/replay quality, and subtracts quick-exit and slow-time-to-interactive penalties. One of every five candidate positions is reserved for a low-impression game when both low- and higher-impression pools exist. The baseline `playtest` pipeline remains an eligibility gate; it is not converted into an unpersisted numeric score.

The For You feed is intentionally simple and explainable. It uses newest-first order until the in-memory index contains at least 20 social events/reactions. After that, the score is:

- Freshness: `1 / (1 + ageHours / 24)`
- Engagement additions: reaction +1; completion +2; replay +1.5; share/save +1.25; quick exit -0.5
- Baseline score: `freshness + 0.03 * engagement`
- Candidate quality adjustments: `+0.20 * smoothed tap rate + 0.18 * smoothed completion rate * duration quality + 0.10 * smoothed replay rate * duration quality - 0.18 * smoothed quick-exit rate - bounded TTI penalty`
- Stable tie-break: newer post first
- Creator diversity: round-robin creator groups while preserving each creator's internal ranking order

Event weights are aggregated once per feed request, so ranking scales as O(events + games log games) instead of rescanning the event list inside sort comparisons. This is a guardrail baseline, not a learned recommender. The event/reaction index is process-local and may be incomplete after a restart; it must not be treated as a durable analytics warehouse. Following/Friends scopes, block, not-interested, muted topics, moderation state, and quarantine filters are applied before ranking.

## Immediate rollback

Set `DQ_FEED_RANKING_ENABLED=false` to force newest-first ordering while retaining all feed safety filters and creator-diversity interleaving. Unset or set the variable to another value to use the normal threshold-based policy. Restart the server after changing the environment.

## Evaluation before tuning weights

Compare a candidate ranking against newest-first on a fixed replayable fixture and, when real traffic is sufficient, a privacy-aware aggregate evaluation. Track:

- qualified plays and completion rate, normalized by impressions where available;
- replay, share, save, and reaction rates;
- quick exits and user feedback such as not-interested/block/report;
- creator and topic diversity in the first page and across pagination;
- moderation/quarantine rates and performance.

Do not optimize raw time spent as the only objective. Keep a stable newest-first control, document each weight change, and retain the rollback switch. Do not tune weights on the basis of a handful of events; the current 20-signal threshold only switches away from cold start and does not prove statistical significance.
