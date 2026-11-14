# Exact-case v2 semantic input wire grammar

Parents: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [canonical input v2 #381](https://github.com/GavinDu/html5-game-generator/issues/381), scoped [#412](https://github.com/GavinDu/html5-game-generator/issues/412).

## Problem and deterministic trust boundary

Go's `encoding/json` struct decoder matches JSON object members case-insensitively, **even when `DisallowUnknownFields` is enabled**. The previous strict decoder checked for duplicate decoded *identical* map keys, so a JSON object containing both `"events":[play inputs]` and `"Events":[]` could pass wire-key uniqueness and the mandatory canonical `events` field presence check, then decode into a different trace because the later case alias overwrote the first value.

The standalone decoder now enforces **exactly spelled, case-sensitive** object member sets before typed decoding:

- Envelope: `schema`, `durationTicks`, `events`.
- Event: `tick`, `ordinal`, `action`, `device`, `phase`, and optional `position`.
- Position: `x`, `y`.

Member names can still use JSON `\uXXXX` escapes if they decode **exactly** to the canonical name, including `"sch\u0065ma"` and `"\u0078"`. Those are not case aliases. Game actions and IR-defined identifiers retain their existing case-sensitive **value** semantics. An extra differently cased key is always an error, even if it supplies the same value. Existing 1 MiB byte limits, unique-key scan, unknown/null rejection, nested depth, fixed-point coordinate bounds, balanced input edges, and v1 historical trace isolation stay in place.

## Adversarial challenge and scope limit

Tests cover top-level alias shadowing that attempts to drop an entire nonempty replay into `"Events":[]`, plus event action/tick/phase/position and coordinate X/Y aliases. Positive tests exercise legitimate canonical escaped member spellings and case-sensitive declared action *values*. Both Chromium and WebKit browser matrices and Go/race/static/PostgreSQL/Playability Kit GitHub Actions must finish SUCCESS on the exact PR head before merging.

This pure parser hygiene **does not** make a browser's inputs truthful; a SHA of client-supplied events is not attestation. Authenticated HTTP v2 input writer/admission, immutable published IR/seed, semantic Go reducer, server-owned score/win verdict and verified-only ranking remain #381/#382/#360/#402. WebKit nonce/session enforcement and physical iPhone/iPad scene presentation/memory are separate Epic #202 gates.

Rollback reverts only the exact-case wire helper, checks, adversarial tests and this documentation.
