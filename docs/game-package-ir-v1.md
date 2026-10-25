# Game Package and Game IR v1

> **Runtime decision (2026-10-20):** PixiJS + Rapier is the single runtime
> engine for Game IR packages. The Defold runtime, Godot endpoints/routing,
> and the native Swift interpreter were removed; statements below that name
> Defold as the runtime target are historical.

Status: implementation in progress. The pipeline now emits and validates Game IR, publishes it on the game record, and serves a versioned Game Package envelope; the PixiJS + Rapier runtime is the remaining implementation step.

## Boundaries

- **GameSpec** captures creator intent and generation choices.
- **Game IR** captures engine-independent gameplay semantics.
- **Game Package** is a versioned artifact: manifest + IR + assets + provenance.
- **Runtime** interprets supported IR using capabilities shipped with the client.
- **Inventory service** owns generation, validation, immutable versions, visibility, moderation and delivery.
- **Hephaestus** owns social UX and selects an installed runtime.

The package is canonical; neither Defold project files nor HTML/JavaScript source is canonical.

## Layout

```text
game-package/
  manifest.json
  game.ir.json
  assets/<asset-id>.<extension>
  provenance.json
```

The manifest carries `packageFormatVersion`, `irVersion`, `minRuntimeVersion`, game/version identity, asset hashes and entry-scene ID. IR references assets by stable IDs; the manifest maps IDs to relative URIs. Provenance includes creator/source and remix lineage. Do not include private source media in public packages unless explicitly published by the creator.

## v1 execution model

V1 supports scenes, entities, allowlisted components, input bindings and declarative event/condition/action rules. It excludes arbitrary Lua, JavaScript, GDScript, native libraries, dynamic imports, eval and arbitrary native API calls. Add capabilities by versioning the contract and shipping a runtime that implements them. Unknown major versions, components and actions must fail closed.

A data-only contract reduces the attack surface but does not guarantee App Review approval. Interpreted Lua is not blanket permission to download arbitrary scripts.

## Validation and publication

```text
creator input -> GameSpec/confirmation -> assets + provenance -> Game IR
  -> schema validation -> semantic/reference validation -> resource budgets
  -> moderation/rights checks -> runtime compatibility -> automated playtest
  -> immutable published version
```

Validate more than JSON syntax: unique IDs, references, supported event/action types, entity/scene/asset limits, media dimensions/duration, hashes, compatibility and moderation status. Keep moderation decisions and social counters outside the package content hash.

## Current backend API

- `POST /v1/game-ir/validate`: validates one Game IR document, rejecting unknown fields, unsupported capabilities and invalid references. Request size is capped at 2 MiB.
- `GET /v1/games/{id}/game-ir`: returns the published canonical Game IR.
- `GET /v1/games/{id}/package`: returns the Game Package envelope with `packageFormatVersion: "2.0"`, `irVersion`, `minRuntimeVersion`, `gameId`, `ir`, and asset descriptors. The player checks the minimum runtime before interpreting the package.
- `GET /v1/games/{id}/assets/{assetId}`: delivers the corresponding package asset after resolving its ID through the validated IR.

The generation pipeline writes `game-ir.json`, validates it before publication, and stores the validated document on the published game. The GameShim `game.js` artifact remains temporary HTML5 reference infrastructure; it is not the canonical package entry point and should be removed after Defold contract tests pass.

## Delivery caching and invalidation

- Public, ready, explicitly moderation-approved, non-quarantined package envelopes and assets return a strong SHA-256 ETag and `Cache-Control: public, max-age=0, must-revalidate`. Conditional requests with a matching `If-None-Match` receive `304 Not Modified`.
- Private, group-restricted, creator-preview, unapproved, quarantined, pending, and rejected delivery responses use `Cache-Control: private, no-store`; they must never enter shared proxy caches.
- Package ETags hash the serialized package envelope. When an asset declares a SHA-256 hash, delivery verifies the bytes against it before returning content; mismatches fail with 422 and `private, no-store`. Asset ETags hash the verified delivered bytes.
- Published package immutability keeps content hashes stable. Public package and asset responses use `max-age=0, must-revalidate`, so compliant caches must revalidate before reuse; restricted responses are `private, no-store`. Republishing changed content under the same published game/version identity is not supported; a changed game must receive a new immutable version/identity. Removed or newly restricted content must be blocked by the delivery authorization check, and deployments with a CDN must purge restricted content when moderation changes. Do not configure stale-if-error or equivalent CDN behavior for game delivery. Use short-lived URLs or a revocable delivery layer if future rights/safety requirements demand revocation.
- Measure package bytes, asset bytes, request latency, 304 rate, and transfer volume before adopting object storage/CDN. Current in-process/file-backed metrics are not a substitute for real traffic measurements, so no CDN is selected by this change.

## Versioning

- Package-envelope version and IR semantic version are separate.
- Published versions are immutable; edits/remixes create a new version.
- Each published version identifies its minimum compatible runtime.
- A remix records its parent game/version and which assets/rules were reused or changed.
- Asset provenance and usage rights must survive remixing.

## Breaking migration plan

Prefer a clean target architecture over indefinite compatibility. Treat the current GameShim JavaScript package format and HTML5/Godot runtime-selection logic as legacy implementations to replace, not permanent public contracts. Coordinate backend and iOS cutover; do not add compatibility adapters unless a concrete migration need justifies their maintenance cost.

1. Make the Game IR schema, semantic validator and golden fixtures authoritative.
2. Change generation to emit Game IR + assets + provenance directly; remove JavaScript code generation from the primary path.
3. Change persistence and API responses to the versioned Game Package envelope. Update backend and client together; breaking API changes are acceptable.
4. Use an HTML5 interpreter only as temporary test infrastructure if useful, not a long-term runtime for compatibility.
5. Implement Defold against the same IR fixtures and run contract tests.
6. After acceptance gates pass, delete GameShim models/endpoints, generated `game.js` artifacts, old runtime-selection branches and obsolete Godot experiments.
7. If legacy published games need migration, write a one-time converter and report unsupported content. Do not maintain a permanent legacy runtime by default.

Game IR defines gameplay semantics. Runtime selection is based on capabilities shipped in the client and required by the package.

## First vertical slice

Use Bobo's Rainbow as the golden fixture: validate it, run it in a temporary HTML5 reference interpreter, then implement the same subset in Defold. Compare input, collision, scoring, completion, determinism and resource use. The reference interpreter is disposable test infrastructure, not a compatibility promise.

## App Store / UGC

The shipped app controls device permissions and native capabilities. The runtime receives only user-authorized media and must not expose arbitrary native APIs. The platform still needs applicable UGC moderation, report/block, rights and takedown workflows. Review Apple's current rules for actual product behavior; no engine choice guarantees approval.

## IR v1.1 — the PixiJS + Rapier expansion (2026-10-20)

Version 1.1 is additive: every v1.0 document remains valid, and the Go
validator accepts both versions. The vocabulary is grounded in the
PixiJS 8.22 and Rapier 0.15 capability inventory (see the research
report `pixijs-rapier-capability-inventory`).

- **Capabilities**: a document may declare the capability names it
  uses (`render.animation`, `physics.joints`, `fx.tween`, …). The
  validator rejects unknown names. Runtimes fail closed — with a
  dev-log warning, never a fake execution — on anything they cannot
  interpret.
- **Scenes** gain `gravity` (logical px/s²), `backgroundColor`,
  `worldSize`, `pixelsPerMeter` (default 100; Rapier works in meters
  internally), parallax `layers`, spawn `templates`, `joints`, and
  ambient particle `emitters`.
- **Rendering** (`entity.render`): tint, alpha, blend mode, anchor,
  scale, flips, visibility, sprite `animation` (frame asset list +
  fps), vector `shape` (rect/rounded/circle/ellipse/polygon/line),
  styled `text`, the five Pixi core `filters` (blur, alpha,
  color_matrix with named operations, noise, displacement), and an
  attached particle emitter.
- **Physics** (`entity.physics`): Rapier body kinds (dynamic, fixed,
  kinematic_position, kinematic_velocity), collider shapes (cuboid,
  ball, capsule, convex, segment), density/friction/restitution with
  combine rules, sensor flag, gravity scale, damping, CCD, axis locks,
  initial velocities, collision groups (16 membership + 16 filter
  bits), active events, sleep, and dominance. `entity.character`
  configures the kinematic character controller (offset, slope angles,
  autostep, snap-to-ground). Behaviors (fall/chase/patrol/oscillate +
  spin/pulse/drift/orbit) drive kinematic bodies; dynamic bodies are
  driven by physics, forces, and impulses.
- **Joints** (scene level): fixed, revolute (limits + velocity/
  position motors), prismatic (axis, limits, motors), rope (max
  length), and spring (rest length, stiffness, damping).
- **Rules** gain events (contact_started/stopped, contact_force,
  entity_destroyed, variable_changed, region_entered/exited,
  animation_complete) and actions (change_health, apply_impulse,
  set_velocity, set_angular_velocity, add_force, set_gravity,
  teleport_entity, set_alpha, set_tint, set_visible, set_asset,
  emit_particles, camera_shake, camera_follow). `spawn_entity` now
  instantiates a scene template for real; `play_sound` and
  `change_scene` execute. Rule `conditions` are evaluated against
  score/health/elapsed time/variables. `timer_elapsed` rules fire on
  their `when.value` second interval. `score_reached` fires when the
  score reaches **or passes** the threshold. Keyboard input bindings
  carry an optional `key` (KeyboardEvent key value); the runtime owns
  keyboard, tween, and particle-emitter behavior because Pixi core
  ships none of them.
- **Recycling**: an entity with `properties.recycle: true` that is
  destroyed or leaves the bottom of the stage respawns at the top in a
  fresh column — the renewable-rain pattern from v1.0, generalized to
  dynamic physics bodies.


## Game Package compatibility envelope

The server response at `GET /v1/games/{id}/package` includes `packageFormatVersion`, `irVersion`, `minRuntimeVersion`, `gameId`, `ir`, `assets`, and `irHash`. The envelope's `irVersion` must match the embedded Game IR document's `irVersion`; clients reject a mismatch or an unsupported IR version before starting the shipped runtime. Current supported IR versions are 1.0, 1.1, and 1.2. The minimum runtime version remains an independent compatibility gate.

Asset SHA-256 values are verified against downloaded bytes before runtime startup. The IR hash identifies the canonical Game IR document; clients must use the same canonicalization contract as the server rather than hashing arbitrary re-serialized JSON.


## Client-side IR compatibility gate

- `packageFormatVersion` versions the outer delivery envelope; `irVersion` versions the semantics of the embedded Game IR. They are not interchangeable.
- The server publishes `irVersion` from the validated IR document. The envelope value must exactly match `ir.irVersion`; missing or mismatched values are invalid.
- The iOS player accepts only its explicitly supported IR versions (currently `1.0`, `1.1`, and `1.2`) and rejects unknown versions before starting the runtime. A package that passes JSON decoding but requires unknown semantics must fail closed with an actionable compatibility error.
- `minRuntimeVersion` remains a separate runtime capability gate. A recognized IR version does not imply the current runtime satisfies the package's minimum runtime version.
- The iOS contract currently checks the shape of the IR hash. Byte-for-byte canonical hash verification must use the exact server canonicalization algorithm before it is treated as an integrity proof; asset bytes are independently checked against each declared SHA-256 before launch.
