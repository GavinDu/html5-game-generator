-- Run inside Defold/Lua test harness. Minimal assertions for the golden fixture.
local runtime_module = require "game_ir"
local doc = json.decode(sys.load_resource("/bobos-rainbow.json"))
local runtime = runtime_module.new(doc)
assert(runtime.score == 0)
runtime:move_player(195, 280)
assert(runtime.score == 1, "overlap should add one point")
assert(runtime.destroyed.heart == true, "collected pickup should be destroyed")
runtime:move_player(195, 280)
assert(runtime.score == 1, "a pickup can only be collected once")
return true
