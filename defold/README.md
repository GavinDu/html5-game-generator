# Defold Game IR runtime prototype

This is the first Defold/Lua interpreter for the engine-independent Game IR contract. It interprets JSON data and a fixed allowlist of declarative actions. It does **not** load or execute downloaded Lua, JavaScript, native libraries, or generated scripts.

## Files

- `runtime/game_ir.lua`: interpreter, state model, bounded rule dispatch, drag movement, pickup overlap, score/health/end-game actions.
- `runtime/main.gui_script.example.lua`: illustrative Defold adapter for input and rendering.
- `runtime/bobos-rainbow.json`: golden fixture copied from the backend contract.

## Integrating into a Defold project

1. Copy `runtime/game_ir.lua` to the project as `/game_ir.lua`.
2. Copy `runtime/bobos-rainbow.json` to `/bobos-rainbow.json` and mark it as a resource in the project.
3. Create a GUI scene and bind touch input to an action named `touch`.
4. Adapt the example GUI script to a stable, predeclared node map for entities and labels. The script deliberately leaves rendering node creation as a project-specific integration point; no full Defold project scaffolding is present in this repository yet.
5. On each input update call `runtime:move_player(x, y)`; render `runtime:visible_entities()`, `runtime.positions`, score, message and result through Defold GUI APIs.
6. Run the shared fixture and compare snapshots and event outcomes with the iOS player.

## Supported in this prototype

- IR version 1.0 and entry-scene selection
- Entity positions, sizes and allowlisted components
- Drag movement, bounding-box pickup detection
- `collected`, `game_start`, `score_reached` event rules
- `add_score`, `set_variable`, `move_entity`, `destroy_entity`, `show_text`, `set_health`, `end_game`
- Callback hook for sound and scene transitions

`spawn_entity` is intentionally a no-op until the contract defines a validated entity template. Unknown actions are ignored. The current adapter is a skeleton, not a finished playable Defold application. Defold engine execution and parity tests still need to be run locally.
