# Feed ranking: baseline, signals, and rollback

## Current policy

The For You feed is intentionally simple and explainable. It uses newest-first order until the in-memory index contains at least 20 social events/reactions. After that, the score is:

- Freshness: `1 / (1 + ageHours / 24)`
- Engagement additions: reaction +1; completion +2; replay +1.5; share/save +1.25; quick exit -0.5
- Combined score: `freshness + 0.03 * engagement`
- Stable tie-break: newer post first
- Creator diversity: round-robin creator groups while preserving each creator's internal ranking order

This is a guardrail baseline, not a learned recommender. The event/reaction index is process-local and may be incomplete after a restart; it must not be treated as a durable analytics warehouse. Following/Friends scopes, block, not-interested, muted topics, moderation state, and quarantine filters are applied before ranking.

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
