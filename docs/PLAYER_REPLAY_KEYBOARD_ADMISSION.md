# Replay keyboard input grammar — fixed 60 Hz run evidence

Epic [Hephaestus #202](https://github.com/GavinDu/Hephaestus/issues/202) · P0 trust boundary [generator #360](https://github.com/GavinDu/html5-game-generator/issues/360).

## Server admission contract

The trusted Pixi runtime records `key`/`keyup` only when the published Game IR declares a `keyboard` input binding whose `key` or `action` exactly matches `KeyboardEvent.key`. The server previously accepted arbitrary `detail` strings for those event types, even from tap-only games. This patch makes **new** replay uploads fail closed unless both down and release match an actual keyboard binding on the server's **current** Game IR. The check runs before run-ID allocation, canonical input SHA calculation, and persistence. Declared-key matching uses exact case and the recorder's alternate action-name match, without a locale-dependent normalization. The existing 128-UTF-16-unit budget, nondecreasing tick constraint, 60 Hz duration horizon and 2,000-event cap still apply.

The Go suite now checks tap-only rejection, synthetic valid keyboard binding, `key` and `keyup` parity, missing/empty/wrong-case input and no run ID on rejection. Historical stored traces are intentionally **not rewritten**; claims and leaderboard values remain unverified, and SHA-256 input evidence does not imply execution. The previously positive tap-only ArrowLeft tests are corrected to reject impossible input rather than encoding a permission bypass as compatibility.

## Design exclusions / follow-on

This is an input vocabulary gate, **not an independent deterministic attestor** or declaration that client reported scores are valid. Backend still must verify immutable package/version/visibility, event completeness, pointer validity relative to controls, same-tick key state and authoritative fixed-step semantic transitions, end state, outcome and score before any `Verified=true` transition. Browser keyup can currently be recorded for a bound key without a prior down, so don't prematurely reject that sequence without synchronizing recorder semantics and versioning the trace protocol. Prove parity across browser engines and physical iPhone/iPad before closing #360 or the epic. No native bearer, auth, cache or WebKit changes.
