# Standardized HTML5 Game Generation Questionnaire and Workflow

**Revision:** repository-aligned v2  
**Repository:** `GavinDu/html5-game-generator`  
**Engine target:** Defold  
**Canonical product artifact:** versioned Game Package containing Game IR, assets, and provenance  
**Legacy artifact:** HTML5 GameShim JavaScript, retained only as temporary reference infrastructure

This revision is based on the repository’s actual README, `RequiredInputs v1.1`, `GameSpec v1`, `game-ir-v1.schema.json`, pipeline, validator, and Defold runtime. No build or runtime execution is assumed.

## 1. What the repository already establishes

The repository already has the right high-level separation:

```text
user uploads / prompt
        ↓
vision and intent extraction
        ↓
RequiredInputs v1.1
        ↓
user confirmation
        ↓
GameSpec / design decisions
        ↓
assets + provenance
        ↓
engine-independent Game IR v1
        ↓
schema + semantic + budget + safety validation
        ↓
versioned Game Package
        ↓
Defold data-only runtime
```

The current generation stages are `spec → user confirmation → art → codegen → validate → publish`. The principal improvement is to make the intermediate contracts authoritative at every boundary and to make Defold the first-class runtime for the canonical IR. The current GameShim output should remain a disposable HTML5 test oracle until Defold contract tests cover the same fixtures.

The repository already contains three related but not identical models:

- `RequiredInputs v1.1`: the rich design questionnaire, including premise, twist, strategy, win/lose, hooks, music, background, characters, features, plot, constraints, and provenance.
- `GameSpec v1`: the compact routing model used for defaults and engine decisions.
- `Game IR v1`: the engine-independent declarative runtime model containing game metadata, assets, scenes, entities, input bindings, rules, and allowlisted actions.

These should not be collapsed into one structure. They serve different purposes and should be explicitly versioned and compiled.

## 2. Revised product principle

The generator should learn transferable patterns from public game research—genre conventions, interaction loops, readable layouts, pacing, feedback, difficulty curves, and win/loss structures—without copying specific games or proprietary assets. The catalog is a pattern reference, not a training set for reproducing character designs, melodies, dialogue, level layouts, source code, or distinctive combinations of expression.

The user’s images, sketches, photos, videos, text, and audio should influence a game through traceable source elements and approved transformations. Every packaged asset needs provenance, rights status, stable ID, hash, and a declared role. The Game Package must not contain private source media unless the creator explicitly publishes it.

## 3. Questionnaire contract

The existing questionnaire in `internal/gamespec/questionnaire.go` is a good creator-facing baseline. Keep its factor-oriented UI, standing preferences, defaults, and enum values. Extend the upstream `RequiredInputs v1.1` document rather than adding dozens of untracked prompt fields to `GameSpec`.

### A. Identity and audience

**Existing fields:** `workingTitle`, `premise`, `constraints.audience`, `constraints.sessionTargetSec`.

Ask:

1. What is the working title?
2. What is the one-line premise statement?
3. Which emotional attitude best describes it: `strange`, `sucks`, `scary`, or `silly`?
4. Who is the audience: children, family, teens, adults, or general?
5. What is the target run length?
6. What should the player feel during the first minute?
7. What is the originality statement: what makes this game distinct?

**Validation:** title and premise are present; premise is no more than 280 characters; target session is within the validator’s current 90-second publish budget unless the budget is intentionally changed in code and documentation.

### B. Source material and rights

**Existing fields:** `sources[]`, `provenance`, `sourceKind`, `artPolicy`.

For each image, sketch, photo, video, text, or audio input ask:

1. What should the system extract: subject, composition, palette, motion, texture, mood, or story cue?
2. Is the material user-created, owned, licensed, public domain, or permission-unknown?
3. Does it contain an identifiable person, brand, logo, copyrighted character, or private location?
4. What transformations are allowed?
5. Should the reference preserve its child-like quality, be polished, or be reimagined?
6. What must not be copied?

**Required improvement:** add a rights-aware source record rather than storing only an upload ID and media kind:

```json
{
  "uploadId": "upload-001",
  "kind": "image",
  "rightsStatus": "owned",
  "consentStatus": "not_applicable",
  "intendedUse": ["palette", "character_concept"],
  "prohibitedUse": ["exact_sprite_copy"],
  "sourceHash": "..."
}
```

### C. Premise and mechanical twist

**Existing fields:** `premise.attitude`, `premise.statement`, `strategy.twist.assumption`, `strategy.twist.reinterpretation`.

Ask:

1. What does the player initially assume will happen?
2. What does the game actually do instead?
3. Is the twist mechanical, narrative, visual, or audio?
4. Is the reinterpretation understandable before it feels unfair?
5. Does the twist support the core loop or merely decorate it?

**Validation:** assumption and reinterpretation are both present and materially different. A twist that merely restates the assumption fails the design gate.

### D. Strategy and core loop

**Existing fields:** `strategy.archetype`, `strategy.coreLoop`, `strategy.input`, `strategy.skillVsLuck`, `strategy.luckSources`, `strategy.difficulty`.

Ask:

1. What is the primary player verb?
2. Which archetype applies: catcher, tapper, dodger, endless runner, one-input physics, physics trials, co-op puzzle platformer, abstract puzzle, territory arena, maze arcade, physics sports, or action platformer?
3. What is the shortest complete loop from input to consequence?
4. What skill improves through practice?
5. What is the intended dominant strategy?
6. What alternative strategies should remain viable?
7. Is the game skill-dominant, mixed, or luck-dominant?
8. If luck exists, does it come from spawns, drops, physics chaos, or none?
9. How many distinct inputs are necessary? Keep the current maximum of two unless the schema is deliberately revised.
10. What is the escalation: speed ramp, hazard escalation, exponential pressure, level arc, or none?
11. Is Level 1 clearable by a simple bot or scripted baseline?

**Design rule:** every feature must declare a `loopLink` explaining how it strengthens the primary loop. This matches the repository’s mechanic-diet validation.

### E. Win, loss, and recovery

**Existing fields:** `winLose.win`, `winLose.lose`, `winLose.failHandling`, `winLose.scoring`, `winLose.progression`.

Ask:

1. What exact event ends a successful run?
2. Is the outcome score attack, level clear, endless high score, time attack, or collection?
3. Is success graded by stars, letter grades, or not graded?
4. What exact event ends a failed run?
5. Is failure a time penalty, instant restart, checkpoint respawn, or no hard failure?
6. What does the player retain after failure?
7. How many actions are required to restart?
8. Can the player understand the reason for the outcome?
9. What happens if the player abandons the run?

**Current repository-aligned limits:** restart should be at most two seconds and one click where possible; the validator currently checks that session target is no more than 90 seconds and that the game is not gated behind a mandatory tutorial.

### F. Hooks and onboarding

**Existing fields:** `hook.timeToInputSec`, `hook.timeToFirstRewardSec`, `hook.firstRewardKind`, `hook.tutorial`, `hook.restart`.

Ask:

1. What is the first meaningful input?
2. How soon must input be accepted?
3. What is the first reward: pickup, score tick, capture, clear, or unlock?
4. How soon should it occur?
5. Is the tutorial none, learn-by-doing, or overlay hints?
6. Is any text mandatory?
7. What is the fastest path from failure back to control?

**Current hard targets:** first input within five seconds, first reward within fifteen seconds, restart within two seconds, and no mandatory pre-input tutorial modal.

### G. Characters and NPCs

**Existing fields:** `characters.player`, `characters.npcs`.

Ask for each entity:

1. What is its role: enemy, friend, neutral, or collectible?
2. What source reference supports the design?
3. What silhouette must be recognizable at small size?
4. What gameplay behaviors does it have?
5. What are its idle, action, hit, success, and failure states?
6. Which characteristics are essential to the user’s idea?
7. Which features are newly generated and original?
8. What accessibility fallback distinguishes it without relying only on color?

`designRef` should point to a stable manifest or source-element ID, not an arbitrary filename. The validator already expects player and NPC design references to resolve against packaged elements or sprites.

### H. Background, layout, and spatial design

**Existing fields:** `visual.background`, `visual.palette`, `visual.effectsBudget`, plus layout information that must be added to the IR.

Ask:

1. Is the background a static image, video loop, procedural field, or parallax layer?
2. Which supplied reference supports it?
3. What is the camera mode: fixed arena, scrolling lane, grid, track, room sequence, or UI-dominant?
4. Where are the safe zones, spawn zones, objective zones, hazards, and UI safe areas?
5. What is the logical canvas size and orientation?
6. What must remain readable on mobile and tablet?
7. What collision and reachability rules constrain the layout?
8. How does difficulty change layout density or spacing?
9. What is decorative and what is gameplay-critical?

**Required IR improvement:** add a validated layout section or encode layout constraints in scene metadata. The current Game IR has positions, sizes, scenes, entities, and components, but no explicit camera, safe-zone, spawn-rule, or layout-constraint model. Without this, layout generation remains hidden in code and cannot be validated or reproduced.

### I. Audio and music

**Existing fields:** `audio.music`, `audio.sfx`, `audio.muteToggle`, `audio.noAutoplayBeforeInput`.

Ask:

1. Is music required, optional, or intentionally absent?
2. What mood and style apply: cheerful, tense, calm, epic, silly, mysterious; chiptune, electronic, orchestral, acoustic, or none?
3. Is it a loop?
4. Which events need sound effects?
5. What happens before first input?
6. Can music and effects be muted independently?
7. Which audio is generated, user-owned, or licensed?
8. What is the audio budget?

The current generated Game IR includes `play_sound`, but the Defold interpreter treats it as a rendering-adapter capability and does not yet play audio. Therefore, audio should remain in the package and IR now, while Defold audio playback is implemented as a capability-backed adapter rather than executable package code.

### J. Features and mechanic diet

**Existing fields:** `features.powerups`, `features.modes`, `features.coop`, `features.leaderboard`, `features.skins`.

Ask for every power-up or mode:

1. What is its name?
2. How does it change the core loop?
3. Which source element or user idea justifies it?
4. What is its duration and counterplay?
5. Does it create a new state, entity, rule, asset, or input?
6. What happens if it is omitted on a low-performance device?

Every feature must include `loopLink`; `sourceRef` should be supplied when the feature is derived from user material. Features without a clear loop link should be removed rather than automatically generated.

### K. Constraints and platform delivery

**Existing fields:** `constraints.sessionTargetSec`, `constraints.packageMB`, `constraints.coldStartSec`, `constraints.audience`, `constraints.perfBudget`, plus `GameSpec.targets`, `orientation`, and `perfBudget`.

Ask:

1. Which targets are required: web, iOS, Android, desktop?
2. Which orientation is required?
3. What is the performance budget?
4. What is the maximum package size?
5. What is the cold-start target?
6. Are external requests forbidden?
7. What save behavior is required?
8. Which browsers and input devices are supported?
9. Is Poki SDK integration required?

The current validator enforces a 25 MB package limit, while `RequiredInputs` also carries a cold-start target that is not yet statically measured. Keep both fields, but distinguish declared budget from measured result.

## 4. Canonical contracts and compilation

### Contract 1: `RequiredInputs v1.1`

This is the creator-facing design document. It should contain the full questionnaire answers, user source references, rights/provenance, and design rationale.

### Contract 2: `GameSpec v1`

This is a compact routing and defaulting model. It should contain only the factors needed to choose a generator/runtime profile and establish defaults. Do not put detailed asset provenance, layout rules, or complex rule graphs here.

### Contract 3: `Game IR v1`

This is the engine-independent gameplay document. The existing schema supports:

- game identity, entry scene, orientation, session target, and audience;
- assets with IDs, URIs, MIME types, hashes, source classes, and alt text;
- scenes and entities with positions, sizes, kinds, components, and properties;
- input bindings;
- declarative event/condition/action rules;
- allowlisted actions including score, variables, movement, destruction, spawning, sound, text, scene change, health, and game end.

The compiler must reject unsupported concepts instead of silently approximating them. For example, a requested camera system, inventory, complex physics, skeletal animation, or network mode must either map to an explicitly versioned capability or be marked unsupported before publication.

### Contract 4: Game Package

Use the package layout already documented by the repository:

```text
game-package/
  manifest.json
  game.ir.json
  assets/<asset-id>.<extension>
  provenance.json
```

The manifest should include package format version, IR version, minimum runtime version, entry scene, asset descriptors, hashes, and compatibility requirements. Published versions must be immutable. Remixes must record parent game/version and changed assets/rules.

## 5. Revised generation workflow

### Stage 0: intake normalization

Normalize uploads, text, sketches, photos, videos, and audio into source records. Generate stable upload IDs and hashes. Extract candidate elements such as character, prop, background, obstacle, and pickup, but label extraction confidence.

**Gate:** all inputs have source IDs, media types, rights status, and intended use. Unknown rights block publication.

### Stage 1: design expansion

An LLM or rules engine expands source elements into `RequiredInputs v1.1`: premise, twist, strategy, win/lose, hook, audio, visual, characters, features, plot, constraints, and provenance.

**Gate:** run `RequiredInputs.Validate()` semantics before asking for confirmation. Do not let the model invent values outside the closed enums.

### Stage 2: human confirmation

Show the design document before art or code generation. The user may edit title, premise, strategy, win/lose, characters, music, and features. Record the confirmed version and the user’s changes.

**Gate:** no downstream generation begins without a confirmed design document. This preserves the repository’s existing pause at `awaiting_confirm`.

### Stage 3: GameSpec compilation and engine routing

Compile the confirmed design into `GameSpec v1`, apply standing preferences only where the user has not answered, and choose a runtime profile. Prefer the Defold IR path for supported declarative mechanics. Keep HTML5 GameShim only as a temporary test oracle.

**Gate:** the spec validates, defaults are explicit, and every requested feature has a target capability.

### Stage 4: asset and media generation

Generate or import assets with stable IDs. Produce a manifest before packaging. Keep user-uploaded source media separate from generated outputs. Generate music and effects only from licensed or approved sources.

**Gate:** every `designRef`, background reference, audio reference, and entity asset resolves to a manifest ID; hashes and provenance exist; package budgets are estimated.

### Stage 5: Game IR emission

Emit deterministic JSON using only the supported IR schema. Generate scenes, entities, input bindings, and rules. Keep behavior declarative. Do not generate or download Lua, JavaScript, GDScript, native libraries, dynamic imports, or eval-based logic.

**Gate:** schema validation, unique IDs, reference resolution, supported components/actions, bounded scene/entity/rule counts, and deterministic output.

### Stage 6: Defold runtime interpretation

The Defold runtime loads validated IR and maps it to rendering, input, audio, and presentation adapters. The runtime owns device-specific behavior; the package owns game semantics. Unknown IR versions, components, actions, or capabilities fail closed.

**Current gap:** the repository’s Defold runtime supports a useful declarative subset, but spawning and sound are currently no-ops and dynamic scene rendering is incomplete. These are project improvements, not questionnaire defaults.

### Stage 7: validation and repair

Run static checks first, then capability checks, resource checks, content screening, and a deterministic playtest harness. A skipped check must remain `skip`; it must never be reported as a pass.

Repair should be bounded. Each repair attempt must produce a new candidate artifact and a diff of the changed IR/assets. After the maximum attempts, stop and request human review.

### Stage 8: publication

Publish only an immutable Game Package whose IR, assets, provenance, validation report, runtime compatibility, and moderation status all pass. Keep social visibility, moderation decisions, and counters outside the package content hash.

## 6. Repository-aligned validation matrix

### Design validation

- `schemaVersion` is exactly the supported `RequiredInputs` version.
- Premise attitude and statement exist.
- Strategy archetype, core loop, twist, primary input, and difficulty ramp exist.
- Win and loss conditions exist and are compatible.
- Time-to-input, time-to-reward, and restart limits pass.
- Every feature has a loop link.
- Player and NPC design references exist.
- Palette values use valid `#RRGGBB` format where supplied.
- Session target, package budget, and audience are declared.

### Asset validation

- Every referenced asset resolves.
- Every manifest file exists.
- Asset IDs are unique and stable.
- Audio is present or silence is explicitly intentional.
- Source, generated, licensed, and platform assets are distinguished.
- No private source media is published accidentally.

### IR validation

- `irVersion` is supported.
- Entry scene exists.
- Scene, entity, asset, rule, and action IDs are unique.
- All references resolve.
- Components and actions are allowlisted.
- Rule dispatch is bounded.
- Scene/entity/rule/action counts remain within schema limits.
- No executable code is present in package content.

### Runtime validation

- Input bindings map to declared actions.
- Every primary action has feedback.
- Success and failure are reachable.
- Failure has a recovery path.
- No state can recurse beyond dispatch limits.
- Unknown capabilities fail safely.
- Audio and scene changes do not leave stale state.
- Optional service failure does not prevent core play.

### Platform and HTML5 validation

- The game is tested over HTTP, not by opening `index.html` directly.
- Defold HTML5 WebAssembly and MIME configuration match the host.
- Responsive orientation and input behavior are tested.
- Package and cold-start budgets are measured separately from declared targets.
- If Poki is a target, add its SDK event ordering, loading, audio, ad, and external-resource checks at the release layer rather than embedding Poki assumptions into core Game IR.

## 7. Project improvements list

### P0 — make the canonical pipeline honest

1. **Make Game IR the only primary code-generation output.** Keep `game.js` only as a temporary reference artifact and remove it after Defold contract coverage is sufficient.
2. **Add one explicit compiler boundary:** `RequiredInputs v1.1 → GameSpec v1 → Game IR v1`. Store each artifact and its schema version in the job record.
3. **Persist the confirmed design input.** The current confirmation step changes a design document, but the canonical `RequiredInputs` and user edits should be stored immutably.
4. **Add provenance to the Game Package.** Include source upload IDs, rights status, transformation policy, generator version, and parent lineage.
5. **Fail closed on unsupported capabilities.** Do not silently turn unsupported mechanics into decorative text or no-op rules.
6. **Separate declared, measured, and skipped checks.** The current validator already models skipped checks; preserve that distinction in the API and UI.

### P1 — strengthen IR and Defold

7. **Implement a versioned capability manifest.** Declare capabilities such as `basic_collision`, `spawn_entity`, `audio_playback`, `scene_change`, `camera_follow`, `timer`, and `touch_input` with minimum runtime versions.
8. **Complete Defold audio adapter support.** Map IR `play_sound` to named sound groups, mute state, gating, and focus/pause behavior.
9. **Implement validated entity templates for `spawn_entity`.** Replace the current fail-closed no-op with a bounded template registry; never allow arbitrary package code.
10. **Add dynamic scene reconstruction.** The current runtime can interpret scene changes but needs a validated visual reconstruction path for multi-scene games.
11. **Add layout and camera schemas.** Make safe zones, camera rules, spawn constraints, collision bounds, and orientation behavior data-driven and testable.
12. **Add stable asset-ID resolution in Defold.** Resolve package IDs to imported resources through a generated map, with missing-asset fallbacks and validation.
13. **Add deterministic seeds.** Store layout/procedural seeds in the package so the same IR produces reproducible playtests.

### P1 — improve the questionnaire and content pipeline

14. **Add rights and consent fields to every upload.** The current `UploadSource` only records ID and kind.
15. **Add explicit camera/layout questions.** The current questionnaire covers visual effects and parallax but not safe zones, camera mode, spawn rules, or reachability.
16. **Add accessibility questions.** Include non-color feedback, readable scale, reduced motion, audio-independent feedback, and touch target size.
17. **Add save policy and privacy fields.** Distinguish no-save, local save, account save, and platform save.
18. **Add an originality-risk review.** Compare abstract mechanics freely, but flag distinctive expression, names, silhouettes, melodies, and source-derived combinations.
19. **Add a source-to-feature trace.** Every power-up, mode, character, and setting should reference either a source element or an explicit original design decision.
20. **Split generated media from licensed library media.** The current pipeline’s music source should resolve before packaging; a manifest entry must not claim a library asset is bundled when it is absent.

### P2 — improve validation and operations

21. **Replace stub vision analysis with a structured extraction result.** Preserve the existing contract of elements plus intent, but add confidence, bounding region, source reference, and ambiguity notes.
22. **Add a headless playtest worker.** Implement the currently skipped checks for time-to-input, first reward, restart, win reachability, softlocks, level-one clearability, difficulty ramp, and memory bound.
23. **Add a browser-based HTML5 smoke test.** Verify loading, resize, pointer/touch input, pause, audio, and terminal states.
24. **Add resource-budget reporting.** Report per-asset and per-scene size, decoded memory, initial-load set, and optional-load set.
25. **Add moderation and rights review as a hard publication gate.** The current content screen is a warning requiring manual review; keep it non-auto-repairable.
26. **Add repair diffs and attempt limits.** Every automatic repair should identify changed fields, assets, and validation results.
27. **Add golden IR fixtures.** Use Bobo’s Rainbow as the first shared fixture, then add one fixture per supported mechanic family.
28. **Add cross-runtime contract tests.** Compare Defold behavior against the temporary HTML5 reference for input, collision, scoring, completion, determinism, and resource use.
29. **Install or provision Go and Defold only in CI or a development environment.** This is not required for the design contract, but it is required before claiming repository-level test coverage.
30. **Remove stale engine-routing assumptions.** The README still describes HTML5/Godot selection as transitional; make the canonical runtime/capability policy explicit and keep legacy paths isolated.

## 8. Standard outputs per generation job

```text
job/<id>/
  source-records.json
  required-inputs.v1.1.json
  confirmed-design.json
  game-spec.v1.json
  game-ir.v1.json
  manifest.json
  provenance.json
  assets/
  validation-report.json
  validation-report.md
  repair-history.json
  runtime-compatibility.json
  build-artifacts/
```

Every file should record schema version, generator version, parent artifact hash, and creation stage. Published artifacts are immutable; edits create a new version.

## 9. Recommended migration sequence

1. Freeze `RequiredInputs v1.1`, `GameSpec v1`, and `Game IR v1` as separate public contracts inside the repository.
2. Add rights-aware source records and the source-to-feature trace.
3. Add layout/camera/accessibility/save fields upstream in `RequiredInputs`.
4. Add capability declarations and fail-closed compilation.
5. Complete one Defold golden fixture end to end using the existing IR runtime.
6. Implement audio, spawn templates, asset-ID mapping, and multi-scene rendering.
7. Add deterministic headless/browser validation.
8. Move the primary publication path from GameShim to Defold-compatible Game Package validation.
9. Retire legacy GameShim and obsolete engine-selection branches only after contract tests pass.
10. Use the Poki-specific checks as a deployment profile, not as hidden assumptions in the engine-independent IR.

## References

[1]: https://github.com/GavinDu/html5-game-generator "GavinDu HTML5 game generator repository"
[2]: https://defold.com/manuals/html5/ "Defold HTML5 development"
[3]: https://defold.com/manuals/input/ "Defold input"
[4]: https://defold.com/manuals/sound/ "Defold sound"
[5]: https://defold.com/manuals/resource/ "Defold resource management"
[6]: https://developers.poki.com/guide/requirements-quality "Poki developer requirements"
[7]: https://developers.poki.com/guide/sdk-html5 "PokiSDK HTML5"
