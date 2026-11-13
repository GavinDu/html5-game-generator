# Canonical pointer replay input shape — Epic #202 / generator #316

**Scope:** a small, fail-closed IR v1.x input-evidence compatibility guard.
It is NOT independent game simulation, server attestation, anti-cheat or a
new action vocabulary.

The preinstalled Pixi runtime records a fixed-step event list, and Go
`POST /v1/games/{id}/runs` validates it before persistence. Both now
require the same shape for legacy pointer events:

- `pointer`: exactly `x,y`, two base-10 canonical signed integers;
  `0` and negative integers are legal, `-0`, leading zeros, plus signs,
  whitespace, decimal points, exponent notation, extra fields, empty
  fields, `NaN` and `Infinity` are not.
- Each coordinate lies inclusively in [-1,000,000, +1,000,000] logical
  pixels. This includes legitimate captured off-canvas drags and denies
  arbitrarily huge numbers. Future world-space semantics need a new
  versioned type rather than silently changing this format.
- `pointerup`: the detail must be exactly the empty string.
- `key` / `keyup`: unchanged <=128 UTF-16 units under v1.x; binding
  names and permitted keyboard combinations require separate versioned
  IR-aware input grammar work.
- Global budgets are unchanged: at most 2,000 ordered fixed-step events,
  nondecreasing ticks, at most 128 UTF-16 units of detail. A trusted
  recorder invalidates its evidence permanently on malformed events;
  gameplay can continue but the trace must not be verified/submitted.
- The Go HTTP boundary returns 400 for a malformed input and leaves
  stored runs unchanged. Existing recorded runs are not silently rewritten.

**Cross-layer evidence:** `internal/api/runs_test.go` and
`tools/playtest/runtime-replay-input.test.mjs` carry equivalent
positive/negative cases. The genuine browser replay adapter and the Go
backend both currently ingest these shapes, but a separate server-owned
replay attestor remains required by generator #360.

**Architect challenger acceptance:** This reduces ambiguous replay evidence,
but it does not justify closing generator #316, #315 or Hephaestus #202.
It cannot prove determinism across physical iPhone/iPad devices or
actual compositor frame readiness. CI must pass on the exact PR head before
merging; testing here does not replace real device validation.
