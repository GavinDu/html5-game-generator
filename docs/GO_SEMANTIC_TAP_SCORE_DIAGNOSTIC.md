# Epic #202 / Go interpreter parent #428 — input-score-tap-v1 atomic slice #432

## Scope and security boundary

The package `internal/semanticreplay` provides a **pure server-owned implementation** of a tiny, explicit **diagnostic-only** Game IR subset. This is the first independent Go semantic step toward #428 and #382; it is *not* callable through any live run/score/leaderboard API, and cannot set a verification verdict.

`SupportedIR(ir)` first calls the canonical Go Game IR validator and then only admits:
- IR v1.0, no modules/experience/capabilities/implicit session timer.
- One scene with zero entities/templates/joints/emitters/layers/gravity/world size or assets/physics; no hidden sprite/entity-specific semantics.
- Declared pointer **tap** inputs only, no drag/keyboard/axes or implicit key repeat.
- Rules with event **input** and explicit bound inputAction only, no entity source/target/value or conditions.
- Actions **add_score** with a finite integral numeric delta in [-1000,1000] or **end_game** with explicit Boolean; no other effects or action-target fields.
- At most 32 rules and 16 actions per rule (canonical IR also validates each), with a total 50,000-work-unit budget.

Every unsupported or malformed field **rejects the whole profile** instead of being silently ignored. This is deliberate because a partial replay could otherwise fabricate a score or outcome.

`ReplayDiagnostic(ir, v2trace, seed)` validates the full canonical edge grammar via the trusted `replaytrace` package (60Hz tick, same-tick ordinals, down/move/up/cancel grammar, held-state completeness, max 2,000 events, <=108000 simulation ticks), then only processes declared pointer-tap **down** actions in canonical order. Release/cancel affects trace validity but is not a second gameplay score event. Results carry a pinned full IR/trace SHA and a stable SHA256 of versioned diagnostic state, with explicit `attested:false`, `evidenceStatus:unverified`, `rankingScope:none`. Runtime never uses a browser-provided score/win verdict. Replaying the same IR/seed/trace is deterministic. Different seeds change evidence identity even if score happens to be identical.

## Senior skeptical game-engine-architect review: exclusions

1. **Not yet a proof of cross-language behavior.** Existing production Pixi has an input-to-entity path, instance-specific target resolution, fixed-tick input FIFO, physics/collision and browser presentation. This profile rejects all entity-bearing games and does not claim all shipped Game IRs work. The next step must create a genuine published zero-entity tap fixture and require Chromium+WebKit identical outcomes before relaxing the profile.
2. The trace and seed originate from the client or diagnostic caller. Pure Go replay is **not** authorization to mark a result verified, attested or ranked. Full published package identity, authenticated client submission, complete execution timing, server-owned seed/admission and signed immutable verdict transition remain #381/#382/#429/#402.
3. This code intentionally rejects UI-driven keyboard repeats, drag/entity target semantics, physics, timers, branching dialogue, dynamic spawn, health/invulnerability and unsupported IR versions. The larger interpreter #428 must add each behind capability negotiation and golden conformance.
4. Unit tests cover empty trace, same-tick ordinal, negative/positive score, terminal stop/no mutation, malformed or incomplete input, invalid/unsupported IR and bounded CPU work. No iPhone/iPad physical frame, 50-cycle memory/thermal or App Store approval is claimed.
5. Rollback is a pure package + tests + docs removal. No production route, installed Pixi game behavior or native WKWebView path changed.

## Acceptance

Only close scoped #432 once all six exact-head Go/race/static, PostgreSQL, Chromium, WebKit, cross-browser determinism and Playability Kit GitHub Actions succeed and a senior engine-architect adversarial source review is recorded. Parent #428, #382 and Hephaestus Epic #202 must remain OPEN pending their own execution/physical-device gates.
