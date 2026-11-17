# Diagnostic Go integer/boolean condition subset — Epic #202 / #465

The preinstalled Pixi interpreter imports `runtime-rule-predicates.mjs`. Its v1.x `conditionsMet` uses JS numeric coercion when RHS is not boolean and both `Number(lhs)`/`Number(rhs)` parse, but strict `===` and `!==` when RHS is boolean. The private unexported `compareReplayScalarV1` in `internal/attestation` matches this **only** for declared bounded integer counters and boolean values.

A boolean LHS with numeric RHS uses `true=1` and `false=0`, including ordering. A boolean RHS uses exact type equality/inequality and never JS numeric ordering. Negative/zero, inclusive boundaries and unsupported operators are tested against the existing JS v1 matrix semantics.

**Deliberate fail-closed subset:** Legacy JS can coerce numeric strings, whitespace, infinity and non-integer floats; the Go version does **not** claim broad compatibility. Strings, fractional/nonfinite/oversized numbers, unknown operators and unsupported value types return `errUnsupportedScalarPredicate`, distinctly from a well-formed predicate evaluating false. The eventual fixed-tick interpreter must first reject a Game IR package whose authored conditions lie outside this declared capability, not skip a condition as false, silently change game outcomes or produce a trusted verdict.

This is pure diagnostic source code and unit matrix: no tick scheduler, no game/trace admission, no CSPRNG issuer, no replay worker, no persistence, no `Verified` status, no public HTTP or score/rank changes. Parent [#428](https://github.com/GavinDu/html5-game-generator/issues/428), [#382](https://github.com/GavinDu/html5-game-generator/issues/382), [#402](https://github.com/GavinDu/html5-game-generator/issues/402) and [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202) remain OPEN.

Acceptance: six current-head Go/race/PostgreSQL/Chromium/WebKit/determinism/Playability Kit jobs all pass, plus a skeptical senior engine-architect source review comparing each accepted scalar case to JS and rejecting unsupported types. No physical iPhone/iPad compositor or App Review certification.
