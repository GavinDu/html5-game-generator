# Strict canonical semantic input v2 JSON decoding — independently testable Epic #202 slice

Related: [Epic #202](https://github.com/GavinDu/Hephaestus/issues/202), [generator #381](https://github.com/GavinDu/html5-game-generator/issues/381), [scope #396](https://github.com/GavinDu/html5-game-generator/issues/396), [grammar #394](https://github.com/GavinDu/html5-game-generator/issues/394).

`internal/replaytrace.DecodeStrictV2` is a **pure preparatory decoder** for the previously merged v2 semantic-input grammar. It is not wired to production `/runs`, does not change the shipped v1 player, and cannot make a run verified. It consumes only a finite UTF-8 JSON value (maximum 1 MiB), refuses excessive depth, repeated decoded object member names (including escaped aliases), trailing JSON, missing/null required fields, unknown nested keys and malformed numeric types. It requires envelope fields `schema`, `durationTicks`, and `events` (an explicit array); each event requires `tick`, `ordinal`, `action`, `device`, and `phase`; an optional `position` must contain both integer `x` and `y`. After decoding, existing `Validate` must admit the complete event sequence against **published** Game IR action bindings. On any error, return no usable trace.

## Adversarial scope and limits

This prevents parser ambiguity from silently rewriting evidence before canonical fingerprinting, but input data and its hash are still untrusted. A future authenticated v2 intake must retrieve immutable authorized Game IR, bind exact full IR digest, seed and physics/runtime capability version, reject incomplete uploads and persist canonical audit evidence. It must not permit old v1 debug traces to become authoritative. That work remains #381/#382/#360. There is still no independently attested game outcome or phone/tablet producer.

Unit tests exercise accepted tap/drag/key and empty event arrays, duplicate top/event/position keys, JSON Unicode-escaped alias keys, unknown fields, null/missing fields, invalid numbers, malformed/trailing bytes, invalid UTF-8, nesting/byte/event limits, and undeclared actions. The merge gate is **all six exact-head GitHub Actions jobs**, not a source-only assertion. This is a source-level senior game-engine-architect challenge; it does not substitute for independent human review or iPhone/iPad testing.

Rollback: revert this isolated unused decoder and test. No production endpoint or runtime behavior changes.
