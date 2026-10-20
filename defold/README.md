# Defold Game IR runtime

This folder is a minimal, runnable Defold project for the canonical Game IR v1.0 contract. The runtime consumes JSON data and allowlisted declarative actions; it never evaluates downloaded Lua, JavaScript, native libraries, or generated scripts.

## Open and run

1. Open this `defold/` folder in the Defold editor.
2. Press **Project → Build** (or Ctrl/Cmd+B), then **Project → Run**.
3. Drag/touch the player box toward the heart. The score should become 1, the heart should disappear, and a message should appear.
4. Press **R** to restart.

## Architecture

- `main/game_ir_runtime.lua`: the single canonical interpreter implementation.
- `main/game_ir.json`: small deterministic fixture for visual smoke testing.
- `main/game.gui_script`: Defold input/render adapter. The current renderer uses generated GUI boxes and labels as placeholders; it does not yet render package image/audio assets.
- `runtime/bobos-rainbow.json`: shared golden fixture copied from `examples/game-ir-v1/bobos-rainbow.json`.
- `runtime/game_ir_test.lua`: smoke assertions intended for a Defold Lua test harness.

The renderer is intentionally separate from game semantics. Next steps are to point the loader at a validated package fetched from the inventory API, build a stable asset-ID-to-resource mapping, and add cross-runtime golden tests.

## Current interpreter support

- Game IR 1.0 and entry scene selection
- Entity positions, sizes and allowlisted components
- Drag/touch movement and bounding-box pickup detection
- `game_start`, `collected`, `score_reached`, `scene_entered` events
- Predicate evaluation for numeric variables (`eq`, `neq`, `gt`, `gte`, `lt`, `lte`)
- `add_score`, `set_variable`, `move_entity`, `destroy_entity`, `show_text`, `set_health`, `end_game`
- Bounded rule dispatch and one-shot score thresholds

`spawn_entity` and audio playback remain no-ops until validated templates and media integration are defined. Scene changes are interpreted by the state engine, but the current fixed-node GUI adapter needs dynamic scene-view reconstruction before multi-scene games are visually supported.

The repository environment does not have the Defold editor/engine, so this project has not been built or run here.
