# Initial renderer-independent Game IR predicate extraction (Epic #202 / generator #317)

This is a **no-intended-behavior-change** vertical slice: the production Pixi
runtime imports trusted `runtime-rule-predicates.mjs` for two evaluation
decisions previously nested inside its giant `run()` closure:
`conditionsMet(rule, state)` and
`ruleMatches(rule, event, source, target, value, inputAction)`.

The module receives plain state/entity data, has no imports and no
DOM/Pixi/Rapier/WebAudio/network/wall-clock access. It preserves the exact
IR v1.x numeric-vs-boolean comparison, reserved score/health/time aliases,
default counter zero, event matching and reference semantics. Deliberately
retained v1.x behavior—including absent source/target being unconstrained
and numeric conversions of some booleans—must be versioned explicitly
rather than changed silently.

Go only serves this **preinstalled trusted module** at a fixed
`/web/runtime-rule-predicates.mjs` route with JavaScript MIME and
`nosniff`. Creator-authored Game IR cannot inject or import modules.
JavaScript golden unit tests cover numeric/boolean/identity edge cases and
verify the real interpreter uses this module. A Go HTTP contract test
confirms its served content and fail-closed unknown module routes.

**Remaining architecture work:** a pure tick/event/command engine, bounded
typed state and event ordering, isolated Pixi and Rapier adapters, complete
resource ownership/disposal, independent server-controlled verification,
native lifecycle authority and real hardware benchmarking. This extraction
does not make Rapier float physics cross-device deterministic; do not claim
rendering independence without complete replay fingerprints.

**Release gate:** only merge after exact-head Go/PostgreSQL, Chromium,
WebKit, Playability Kit and cross-browser determinism all complete
successfully, and senior game-engine-architect review confirms no inventory
game semantic or screenshot drift. Keep #317 and Epic #202 OPEN.
