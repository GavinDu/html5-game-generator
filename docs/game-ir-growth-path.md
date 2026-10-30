# Game IR growth path

Status: proposal only. This document does not add schema fields or claim runtime support. A capability is eligible for implementation only when schema, semantic validation, the PixiJS + Rapier runtime, deterministic replay, and automated playtest land together.

## Product gaps to track

The current declarative vocabulary is effective for arcade and physics-driven games, but several experience families need explicit primitives rather than generated scripts:

| Experience family | Missing or incomplete semantic concepts | Candidate IR direction |
| --- | --- | --- |
| Story and dialogue | branching conversations, speaker identity, dialogue choices, persistent story flags | versioned dialogue graphs and typed story variables |
| Turn-based tactics | turns, phases, legal action sets, initiative, grid movement, action points | deterministic turn controller and board/grid semantics |
| Strategy and management | resources, costs, production, ownership, objectives, unlock conditions | typed resource state, transactions, objective predicates |
| Animation and authored sequences | timelines, keyframes, easing, event markers, cancellation and pause semantics | fixed-time animation tracks and deterministic markers |
| Save/resume | durable player progress, checkpoint identity, schema migration | explicit save-state schema with versioned migration rules |
| Accessibility | remappable actions, text alternatives, timing assistance, reduced-motion variants | semantic input/action metadata and presentation preferences |

This is a gap inventory, not a commitment to implement every row. Prioritize by creator demand and by whether the experience can be expressed safely and deterministically.

## First proposed addition: dialogue graphs

Dialogue is a useful first vertical slice because it adds a recognizable experience family without introducing arbitrary computation or changing the physics model.

### Proposed semantics (not yet a schema contract)

- A dialogue graph has a stable ID, an entry node, and uniquely identified nodes.
- Node kinds are limited to authored line, choice, conditional branch, and terminal node.
- A line references a speaker ID and a bounded text payload; optional portrait/media references use existing asset IDs.
- A choice contains a bounded ordered list of labeled options, each targeting an existing node.
- Conditional branches use the existing typed comparison/variable vocabulary; no expressions, scripts, reflection, or dynamic property paths.
- Story flags and counters use explicitly declared, typed variables. Writes are limited to allowlisted set/increment operations.
- Every edge target must exist. The validator rejects duplicate IDs, unreachable required nodes, invalid entry nodes, unbounded choice lists, unsupported operators, and references to undeclared variables.
- Runtime evaluation is single-step and event-driven. Given the same initial state and player choice sequence, the graph must produce the same node sequence and final variables.
- Dialogue state is included in deterministic replay traces. A replay records selected choice IDs, not localized display strings.
- Text localization is presentation data and must not affect graph identity, branching, or replay hashes.

### Delivery gates

1. **Schema:** add an explicitly versioned dialogue construct; preserve validation of all existing 1.0–1.2 fixtures.
2. **Validator:** enforce node/edge integrity, limits, variable typing, allowlisted operators, and asset-reference rules. Unknown node kinds and operators fail closed.
3. **Runtime:** implement graph traversal, choice presentation hooks, typed variable updates, pause/resume, and deterministic state transitions in PixiJS + Rapier host runtime.
4. **Replay:** add golden traces for a linear conversation, branching choices, conditional branch, terminal state, and invalid/cyclic/unreachable graph cases. Replaying an identical choice trace must produce identical semantic state.
5. **Playtest:** add worker checks for dialogue boot, choice availability, valid transition, terminal completion, and no-softlock. A missing or inconclusive hard check must fail closed.
6. **Compatibility:** update package capability negotiation and web/iOS golden fixtures together. A client that lacks dialogue support must reject the package with an actionable unsupported-capability result before launch.

No dialogue schema version should be published until all six gates pass in CI.

## Determinism and safety invariants

- Game IR remains data-only. Do not add eval, embedded scripts, arbitrary expressions, dynamic imports, downloaded native code, or unrestricted network access.
- All time-dependent behavior uses host-controlled virtual time or fixed simulation steps; never wall-clock timers for gameplay semantics.
- Randomness, if later required, must use a host-provided seeded generator whose state is part of replay.
- Bound graph size, node count, text length, choice count, transition count per tick, and total state size.
- Validation must happen before publication and again at the package trust boundary. Unsupported major versions/capabilities fail closed.
- Runtime and validator support are a single release contract: schema-only additions are not publishable.

## Criteria for revisiting a sandboxed JavaScript escape hatch

Reconsider only when all of the following are true:

1. Multiple concrete creator requests cannot be represented by a reasonably bounded declarative IR extension.
2. A written threat model demonstrates why a declarative addition is insufficient and identifies the exact capability gap.
3. The execution environment is isolated on a separate origin/process boundary with no ambient credentials, cookies, native bridge, filesystem, or privileged host objects.
4. Network access is disabled by default and remains unavailable to game code; any future exception requires a separately reviewed allowlist and mediation layer.
5. CPU, memory, instruction/time, storage, and output budgets are enforced by the host and terminate runaway execution.
6. Randomness and time are host-provided seeded/virtual services; replay records their seed and inputs.
7. The code is immutable, content-addressed, scanned, moderated, and subject to explicit creator rights and takedown controls.
8. The App Store, privacy, moderation, and security implications are reviewed against actual shipped behavior before implementation.

If any invariant cannot be enforced and tested, keep the declarative Game IR approach and extend it instead.

## Rollout and acceptance

- Keep the current Game IR v1.0–1.2 contract unchanged while this proposal is reviewed.
- Prototype dialogue in a separate branch behind fixtures; do not emit it from production generation until validator, runtime, replay, playtest, and compatibility checks all pass.
- Add the same golden package fixtures to backend and client contract suites.
- Document unsupported-capability errors and migration behavior before incrementing the IR version.
- Close the tracking issue only after the first addition has schema, validator, runtime, replay, and playtest coverage. This document alone completes only the planning/documentation portion.
