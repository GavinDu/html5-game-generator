# Fixed-step replay input tick horizon — Epic #202 / server attestation #360

## Confirmed admission gap

`POST /v1/games/{id}/runs` already enforced ordered nonnegative
`RunInput.Tick` values and a maximum of 2,000 inputs, but had **no upper
tick bound**. A client could submit a one-second `durationMs` and an event
at tick 1,000,000,000. Such an event cannot affect the claimed fixed-step
run. Persisting it also creates ambiguous out-of-range input evidence and
a risk for an eventual server replay executor that tries to advance to
an unbounded tick.

The trusted shipped Pixi runtime increments `play.simStep` **once per
fixed 1/60s tick** while `state.elapsed` advances by the same fixed
time. It submits `durationMs=Math.round(state.elapsed*1000)` and the
recorded input ticks together at run end.

## Incremental P0 hardening

- Bound each input tick to `floor(durationMs*60/1000)+60` (one additional
  second of legacy slack). The requested duration is itself already
  constrained to 1s–30m, so the maximum accepted tick is 108,060.
- Return HTTP 400 before allocating the new run identity or writing
  to `runs.jsonl` when an input is beyond that horizon.
- Keep the existing same-tick ordering, supported type/UTF-16 detail,
  pointer canonical-coordinate rules, package-version hash, score
  plausibility and 2,000-input cap unchanged.
- Go HTTP regression covers zero tick, the maximum allowed boundary,
  one-past-boundary rejection for 1s and 20s runs, an extreme future tick,
  and confirmation that rejected traces never receive a run ID or become
  in-memory accepted runs.

## Senior game-engine architect challenger / limitations

This is a cheap **resource and provenance sanity bound**, not
proof that the client's duration, inputs, score, win/outcome or seed are
true. The extra one-second slack is intentionally conservative for older
browser traces; a future versioned replay grammar should make the exact
terminal tick mandatory and derive duration from **server-owned**
simulation, not rely on client-provided `durationMs`.

Any real fixture generating a tick above the bound must be investigated
for inconsistent clocks, pausing or version skew before broadening the
budget. Test representative Pixi games in Chromium/WebKit and the
Playability Kit; a PR with any failed exact-head test must not merge.

**Release gate:** All six Go validation jobs successful (Go formatting,
unit/race, PostgreSQL, Chromium, WebKit, Playability Kit and
cross-browser determinism), source challenger review, and no loss of
previous IR version acceptance. Independent server replay verifier
[#360](https://github.com/GavinDu/html5-game-generator/issues/360)
and iPhone/iPad actual GPU-memory/thermal evidence (#202) remain OPEN.
