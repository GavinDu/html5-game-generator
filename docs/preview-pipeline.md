# Deterministic preview selection

The browser runtime exposes a bounded `events` array in its playtest probe report. Events carry a millisecond timestamp from the visible run clock and a type such as `first_input`, `first_reward`, `score_change`, `collision`, `milestone`, `win`, or `lose`.

`internal/preview.PickWindow` selects a six-second window without changing gameplay or replay verification. The first `first_input` event anchors the fallback. If a meaningful event exists after that interaction, the picker chooses the highest-priority event (`win`/`lose`, score change, first reward, collision, then milestone), breaks ties by earliest timestamp, and centers the window slightly before it. Short runs are clamped to their complete duration, and a run with no meaningful events falls back to the first six seconds after interaction.

The runtime records score changes and hazard collisions in addition to the existing win, lose, first-input, reward, and milestone events. The picker is pure and deterministic: identical event traces and run durations always produce the same `{start_ms, end_ms}` result. The renderer can use that result as the replay interval for issue #100; until that renderer is connected, preview generation continues to use the existing poster/clip workflow.
