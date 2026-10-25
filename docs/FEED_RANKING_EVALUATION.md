# Feed ranking evaluation protocol

Evaluate the current explainable feed ranker against newest-first using identical candidate sets and filter state. This is a repeatable offline evaluation, not a learned recommender launch gate. Do not change ranking weights from a single fixture or small production sample.

## Evaluation contract

Compare:
- **Baseline:** newest-first, with existing stable tie-break behavior.
- **Candidate:** engagement ranking behind the existing feature flag, including the freshness term, documented event weights, reaction counts, and creator-diversity round-robin.

Apply visibility, Following/Friends scope, block, not-interested, muted-topic, moderation, and quarantine filters before either ranking mode. Candidate eligibility must be identical.

## Deterministic fixtures

Version-control fixtures with fixed timestamps and explicit event/reaction records. Include:

1. **Cold start:** no engagement events; candidate falls back to newest-first.
2. **Balanced engagement:** completions, replays, shares, saves, and quick exits across several creators.
3. **Single-creator burst:** one creator has many high-weight events while others have modest engagement.
4. **Freshness conflict:** a newer low-engagement game competes with an older highly engaged game.
5. **Noisy events:** unknown kinds and events without a game ID do not change scores.
6. **Sparse creator pool:** fewer creators than the diversity window; output stays deterministic.
7. **Filter interaction:** blocked, private, quarantined, or otherwise ineligible games never reappear after diversity interleaving.
8. **Tie and duplicate timestamps:** stable tie-breaks are repeatable.

Each fixture states candidate IDs, creator IDs, timestamps, reactions, events, expected eligibility, and expected order or bounded acceptable outcomes. Use a fixed clock or inject evaluation time so wall-clock drift cannot change expected scores.

## Metrics

Report baseline and candidate values per fixture and in aggregate:
- **Engagement quality:** completion, replay, share/save, quick-exit rates, and normalized weighted engagement per impression when impression data exists.
- **Creator diversity:** unique creators in top 10 and top 20; largest creator share in top 10 and top 20; creator Herfindahl-Hirschman index (sum of squared creator impression shares).
- **Freshness:** median age in top 10/top 20; share of top 20 published in the last 24 hours.
- **Safety/correctness:** ineligible results (must be zero), duplicate IDs (must be zero), and deterministic output for identical fixture input.
- **Performance:** candidate count, event count, ranking time, and allocations if supported by the harness.

Do not interpret event counts as causal quality without impression denominators. Without impression data, label event totals as proxy metrics and do not claim a measured engagement-rate lift.

## Acceptance gates

- Deterministic tests pass repeatedly.
- No eligibility/safety regression and no duplicate IDs.
- Cold-start behavior matches newest-first.
- Creator diversity does not materially regress; document and review any trade-off.
- Engagement improvement repeats across balanced fixtures, not only the single-creator burst.
- Record performance at representative candidate/event sizes and retain feature-flag rollback.
- Publish fixture version, test command, metric table, and decision (ship, revise, or roll back) in the PR.

## Suggested result table

| Fixture | Mode | Top-10 unique creators | Largest creator share | Median age | Engagement proxy | Ineligible results | Runtime |
|---|---|---:|---:|---:|---:|---:|---:|
| balanced | newest-first | | | | | | |
| balanced | engagement | | | | | | |
| single-creator burst | newest-first | | | | | | |
| single-creator burst | engagement | | | | | | |

Populate values from the harness; never hand-fill estimates. This document specifies the protocol and does not claim the benchmark has already run.
