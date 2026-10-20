-- Smoke test for the canonical Defold Game IR module.
-- Run inside Defold or a compatible Lua harness with json/sys resource APIs.
local runtime = require "main.game_ir_runtime"
local raw = sys.load_resource("/main/game_ir.json")
assert(raw, "missing /main/game_ir.json")
local ir = json.decode(raw)
local state = runtime.new(ir)
state.emit("game_start")
state.move_player(195, 280)
assert(state.score == 1, "pickup overlap should add one point")
assert(state.destroyed.heart == true, "collected pickup should be removed")
state.move_player(195, 280)
assert(state.score == 1, "a pickup must only count once")
state.move_player(195, 650)
state.move_player(195, 280)
assert(state.score == 1, "destroyed pickup must not re-trigger")
return true
