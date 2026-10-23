# Calibration set — fused scorecard reference cards

The three shipped games, frozen at the IR hashes and runtime version
recorded in each card. These cards are the anchors for judge
stability: any change of judge model, judge prompt, or rubric must
re-score this set from the same evidence packs, and a mean absolute
drift over 7 points on any dimension blocks adoption until a human
reviews the deltas.

Produced 2026-10-22 by the fused protocol (scorecard schema 1.2):
objective dimensions graded from the CI playtest report of run
39887049881; creative dimensions from two blind judge passes
(Muse Spark 1.3 subagents, evidence packs only — no check statuses
or scores visible), merged by `tools/playtest/score-report.mjs`.
No dimension was contested (all pass spreads ≤ 15).

Honest reading: none of the three games meets the delivery target
(overall ≥ 80, every metric ≥ 70) — these are calibration anchors,
not passes. The systemic finding is `audio_feedback` (24–25 on all
three): the games declare almost no sound feedback, a platform-level
gap in the IR feedback vocabulary, not three independent failures.
