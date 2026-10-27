# Feed ranking evaluation results (fixture version 1)

Protocol: `docs/FEED_RANKING_EVALUATION.md`. Harness:
`internal/api/feed_ranking_evaluation_test.go` (`TestFeedRankingEvaluation`).
Base commit: `f91fe769` (main, includes #167). Run date: 2026-10-27.

Test command (repeatable, no network, no live services):

```sh
go test ./internal/api -run 'TestFeedRankingEvaluation' -v -count=1
```

Baseline mode is `DQ_FEED_RANKING_ENABLED=false` (newest-first plus
creator-diversity round-robin). Candidate mode is the flag `true` (quality
score plus exploration plus diversity). Both modes run the real feed handler
on identical candidate sets and filter state. Engagement totals without
impression data are labeled proxy, per the protocol: no engagement-rate lift
is claimed from them.

## Metric table

Top-10 engagement proxy is the sum of legacy event weights
(completion 2, replay 1.5, share/save 1.25, quick exit −0.5) over the top 10,
with the per-impression value when the fixture has impression events.
Runtime is end-to-end handler time for one feed request in the final run;
handler times varied 0.3–1.3 ms across runs for these ≤15-candidate fixtures.
Scale benchmarks (existing `BenchmarkFeedRanking`): 100 games 1.30 ms/op,
1000 games 11.1 ms/op (2× events), so ranking cost is not a concern at
representative sizes.

| Fixture | Mode | Top-10 unique creators | Top-20 unique creators | Largest creator share (top 10) | HHI (top 10) | Median age top 10 (h) | 24h share top 20 | Engagement proxy (top 10) | Ineligible | Duplicates | Deterministic | Runtime (ms) |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| cold-start | newest-first | 3 | 3 | 0.33 | 0.333 | 15.5 | 0.50 | 0.00 (proxy, no impr) | 0 | 0 | yes | 1.06 |
| cold-start | engagement | 3 | 3 | 0.33 | 0.333 | 15.5 | 0.50 | 0.00 (proxy, no impr) | 0 | 0 | yes | 1.27 |
| balanced | newest-first | 4 | 4 | 0.30 | 0.260 | 22.5 | 0.42 | 68.00 (0.680/impr) | 0 | 0 | yes | 1.07 |
| balanced | engagement | 4 | 4 | 0.30 | 0.260 | 22.5 | 0.42 | 68.00 (0.680/impr) | 0 | 0 | yes | 0.82 |
| single-creator-burst | newest-first | 5 | 5 | 0.20 | 0.200 | 6.5 | 0.67 | 149.50 (proxy, no impr) | 0 | 0 | yes | 0.84 |
| single-creator-burst | engagement | 5 | 5 | 0.20 | 0.200 | 6.5 | 0.67 | 149.50 (proxy, no impr) | 0 | 0 | yes | 0.81 |
| freshness-conflict | newest-first | 3 | 3 | 0.40 | 0.360 | 20.0 | 0.60 | 80.25 (proxy, no impr) | 0 | 0 | yes | 0.79 |
| freshness-conflict | engagement | 3 | 3 | 0.40 | 0.360 | 20.0 | 0.60 | 80.25 (proxy, no impr) | 0 | 0 | yes | 0.73 |
| noisy-events | newest-first | 3 | 3 | 0.40 | 0.360 | 15.0 | 0.60 | 37.75 (proxy, no impr) | 0 | 0 | yes | 0.71 |
| noisy-events | engagement | 3 | 3 | 0.40 | 0.360 | 15.0 | 0.60 | 37.75 (proxy, no impr) | 0 | 0 | yes | 0.62 |
| sparse-creators | newest-first | 2 | 2 | 0.50 | 0.500 | 21.5 | 0.50 | 72.50 (proxy, no impr) | 0 | 0 | yes | 1.06 |
| sparse-creators | engagement | 2 | 2 | 0.50 | 0.500 | 28.5 | 0.50 | 88.50 (proxy, no impr) | 0 | 0 | yes | 0.93 |
| filter-interaction | newest-first | 2 | 2 | 0.50 | 0.500 | 15.2 | 0.88 | 64.00 (proxy, no impr) | 0 | 0 | yes | 1.18 |
| filter-interaction | engagement | 2 | 2 | 0.50 | 0.500 | 15.2 | 0.88 | 64.00 (proxy, no impr) | 0 | 0 | yes | 0.86 |
| tie-timestamps | newest-first | 2 | 2 | 0.50 | 0.500 | 5.0 | 1.00 | 0.00 (proxy, no impr) | 0 | 0 | NO | 0.74 |
| tie-timestamps | engagement | 2 | 2 | 0.50 | 0.500 | 5.0 | 1.00 | 0.00 (proxy, no impr) | 0 | 0 | NO | 0.78 |
| preview-quality | newest-first | 6 | 6 | 0.17 | 0.167 | 9.0 | 0.83 | 30.50 (0.508/impr) | 0 | 0 | yes | 0.73 |
| preview-quality | engagement | 6 | 6 | 0.17 | 0.167 | 9.0 | 0.83 | 30.50 (0.508/impr) | 0 | 0 | yes | 0.74 |
| exploration | newest-first | 10 | 15 | 0.10 | 0.100 | 5.5 | 1.00 | 30.00 (0.600/impr) | 0 | 0 | yes | 0.74 |
| exploration | engagement | 10 | 15 | 0.10 | 0.100 | 7.0 | 1.00 | 36.00 (0.450/impr) | 0 | 0 | yes | 0.74 |

Aggregate (mean across fixtures): top-10 unique creators 4.00 both modes;
largest share 0.34 both; median age 13.6 h newest-first vs 14.4 h engagement;
24 h share 0.70 both; weighted engagement per impression 0.596 newest-first
vs 0.546 engagement, averaged over the 3 fixtures with impressions only.

## Per-fixture observations

- Cold start: candidate order is byte-identical to baseline (newest-first
  plus diversity), as required. The 20-signal threshold is not the only
  guard: with zero signals the quality score reduces to freshness anyway.
- Balanced: candidate reorders within the set (highest-engagement game
  first) but the top-10 set is unchanged, so the proxy is identical. This is
  the diversity round-robin at work: with 4 creators × 3 games, every game
  fits in the top 10 regardless of score. The ranker changes order, not
  membership, at this depth.
- Single-creator burst: the burst creator's top game leads the candidate
  feed despite being the oldest, but diversity holds: top-10 unique creators
  and largest share are identical to baseline. No creator takeover.
- Freshness conflict: candidate ranks the 72 h highly engaged game first;
  baseline ranks the 1 h low-engagement game first. Engagement outweighs a
  71 h freshness gap at these weights, as designed.
- Noisy events: candidate and baseline orders are unchanged when 30
  unknown-kind events, 10 game-less events, and 10 player-diagnostic events
  are added. Unknown kinds contribute no score; game-less events are skipped
  by both aggregators.
- Sparse creators: the clearest composition win. Candidate top-10 weighted
  engagement is 88.50 vs 72.50 (+22%) because within-creator score order
  promotes older high-engagement games into the round-robin window.
- Filter interaction: all 9 ineligible games (private, quarantined,
  pending-moderation, rejected-moderation, draft, both block directions,
  not-interested, muted-topic) are absent in both modes, including after
  diversity interleaving and exploration. All 8 eligible games present.
- Tie timestamps: **the only failing gate (see below).**
- Preview quality: candidate ranks the fast high-completion game first and
  demotes the slow quick-exit game below three fillers, despite the slow
  game being 5 minutes newer. Tap-rate, completion-quality, quick-exit, and
  TTI penalties all move the order in the expected direction.
- Exploration: candidate slots 5, 10, and 15 are low-impression games, as
  specified. Candidate top-10 weighted total rises 30.00 → 36.00, but the
  per-impression value falls 0.600 → 0.450 because the candidate top 10
  carries more impressions (80 vs 50) in its denominator. This is a mix
  effect, not a quality regression, and it is why no rate lift is claimed.

## Acceptance gates

- Deterministic tests pass repeatedly: **FAIL — tie-timestamps only.**
  7 distinct orders over 20 runs in both modes. Cause is pre-existing and
  outside the ranker: `MemoryStore.ListGames` sorts with unstable
  `sort.Slice` over Go map iteration, so equal-`CreatedAt` games arrive in
  random order and both the baseline and the candidate inherit it. The
  baseline fails identically, so this is not a ranker regression, but the
  gate as written is not met. Recommended fix (separate change, Gavin's
  call): stable sort plus an ID tie-break in `ListGames` or in the feed
  sort. No production code was changed in this branch.
- No eligibility/safety regression, no duplicates: **PASS** on all
  fixtures, both modes (0 ineligible, 0 duplicates everywhere).
- Cold-start behavior matches newest-first: **PASS** (identical order).
- Creator diversity does not materially regress: **PASS** — top-10 unique
  creators and largest share are identical between modes in every fixture.
- Engagement improvement repeats: **PARTIAL, honestly mixed.** It repeats
  in sparse (+22%), exploration (+20% total), and directionally in
  freshness-conflict and preview-quality. It does not change top-10
  composition in balanced or burst, where diversity fixes the set; and the
  aggregate per-impression value is lower for the candidate (0.546 vs
  0.596) due to the exploration denominator mix. No weight change is
  justified by these fixtures, and none was made.
- Performance and rollback: **PASS.** Sub-millisecond handler times at
  fixture scale; 1.30 ms/op at 100 games, 11.1 ms/op at 1000 games. The
  `DQ_FEED_RANKING_ENABLED=false` rollback is untouched.

## Decision: SHIP (keep enabled; no rollback, no weight changes)

The candidate ranker is safe to keep enabled: safety and diversity gates
pass, cold start is exact, engagement moves in the designed direction where
it can change membership, and performance is fine. Rolling back would not
fix the one failing gate, which affects newest-first identically. Do not
tune weights from these 10 fixtures. The tie-break stabilization is the
only follow-up, as its own small change, not bundled with this evaluation.

## Addendum 2026-10-27: determinism gate now passes (via #168)

The tie-timestamps determinism failure recorded above was fixed after this
run by PR #168 (merged as c3946622): `ListGames` now sorts by a total order
(CreatedAt desc, game ID asc), the feed sort carries the same ID tie-break,
and feed scores are computed once per game from a single request timestamp
(the per-comparison `time.Since` call was making identical games differ by
microseconds, which defeated the tie-break). Re-running
`TestFeedRankingEvaluation` on main c3946622 passes, including the
tie-timestamps fixture. The original run's values above are preserved as
measured; only the determinism gate verdict changes, from FAIL to PASS.
