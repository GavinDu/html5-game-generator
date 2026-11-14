# Canonical semantic input trace v2 — grammar-only extraction

Parent [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [generator input protocol #381](https://github.com/GavinDu/html5-game-generator/issues/381), scoped [#394](https://github.com/GavinDu/html5-game-generator/issues/394).

## Separate pure data contract

`internal/replaytrace` provides a pure Go, renderer/browser/native-independent *future* input protocol. It does **not** change published Game IR, enable a new runtime capability, accept a v2 run upload, or mark any score verified. Only currently declared Game IR `tap`, `drag` and `keyboard` bindings can pass. New controls must roll out together through IR version, client adapter, server intake and evaluator.

The envelope is `{schema:"semantic-input-trace-v2",durationTicks,events:[...]}`. Events contain integer `tick`, 0-based same-tick `ordinal`, declared `action`, `device` (`pointer`/`keyboard`), `phase` (`down`/`move`/`up`/`cancel`) and optional `position:{x,y}` in fixed 1/1000 logical pixel. No OS event names, wall-clock timestamps, floats, dynamic JS, or user-controlled executable code.

- 60 Hz tick horizon bounded to 30 minutes, at most 2,000 events, bounded action names and fixed coordinates.
- Valid pointer tap down/up or cancel, pointer drag down/move/up or cancel; declared keyboard down/up/cancel. Each held action must terminate. Input arrows/keys normalize to a declared semantic action, not native hardware keys in the submitted trace.
- Tick must be nondecreasing; within each tick ordinals start at zero and increase exactly once per event. Same-tick ordering changes replay identity.
- `CanonicalSHA256` hashes game ID, immutable full IR SHA, seed and validated canonical trace; nil and empty event lists are normalized, avoiding byte-identity drift for idle games.
- This scheme does not model multi-touch fingers, pressure, pinch, virtual axes, gamepad or viewport transport transforms yet. Those require negotiated capability changes; silently accepting their names would make broken games publishable.

## Architect review and release exclusions

This is *syntax and semantic input state-admission*, not physics/scene simulation or browser event equivalence. A browser could lie about the data submitted. A matching SHA is not a signature and cannot grant verified leaderboard status. The current production `RunInput` v1 remains unchanged. Full end-to-end input-v2 admission requires a generator browser writer, backend /runs protocol versioning, native iPhone/iPad interaction smoke and independent Go reducer (#381/#382/#360). Do not pretend that tests of this pure package constitute phone/tablet gameplay proof.

Exit criteria for this scoped extract: `go test`/race and negative fixtures, plus required PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit exact-head CI jobs. No skipped tests or requirement relaxations.