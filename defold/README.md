# Defold Game IR runtime (first vertical slice)

This is a Defold project scaffold for the canonical Game IR 1.0 contract in the backend docs.

## Open and run

1. Open Defold.
2. Choose **Open Project** and select the `defold/game.project` file.
3. Build and run. The bundled `main/game_ir.json` is the deterministic fixture.
4. Drag the player box onto the heart. Score and the status message should update. Press **R** to restart.

## Implemented

- Loads JSON Game IR from a bundled resource.
- Resolves the entry scene and builds GUI nodes from entity positions/sizes.
- Touch/mouse drag moves the first player/movement entity within a logical 390 × 844 playfield.
- Collision checks collect pickup/collectible entities once.
- Executes a fixed allowlist for `add_score`, `destroy_entity`, `show_text`, `set_variable`, `move_entity`, `set_health`, and `end_game`.
- Unknown IR versions fail closed; downloaded code is never executed.

## Not implemented in this first slice

Colored placeholder nodes are used; image/audio decoding and downloads are not implemented yet. Scene transitions, spawn, sound, animation, tilemaps, remote package fetching, hash verification, and moderation are subsequent tasks. The backend's package endpoint serves assets by ID. A future Defold loader must resolve only listed asset URLs, validate hashes/size/type, and use a vetted image-decoding path before creating dynamic textures.

The fixture is a quick manual-test game, not a new package format. Keep the backend schema and golden fixture authoritative.

No Defold editor/build tool is available in this editing environment, so please build this project locally in Defold and report any editor/compiler errors for the next pass.
