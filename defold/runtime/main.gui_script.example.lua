-- Example Defold GUI script adapter for game_ir.lua.
-- Add a GUI scene with a full-screen box node named "background", then wire
-- visual nodes/factories to the callbacks below. Downloaded content remains data.
local game_ir = require "game_ir"

function init(self)
    msg.post(".", "acquire_input_focus")
    local raw = sys.load_resource("/bobos-rainbow.json")
    assert(raw, "Missing /bobos-rainbow.json in game resources")
    local doc = json.decode(raw)
    self.runtime = game_ir.new(doc, {
        state_changed = function() self.dirty = true end,
        score_changed = function(score) self.score = score; self.dirty = true end,
        show_text = function(text) self.message = text; self.dirty = true end,
        game_ended = function(result) self.result = result; self.dirty = true end,
        reset = function() self.dirty = true end
    })
    self.dirty = true
end

function on_input(self, action_id, action)
    if action_id == hash("touch") and action.pressed then
        self.dragging = true
        self.runtime:move_player(action.x, action.y)
    elseif action_id == hash("touch") and action.released then
        self.dragging = false
    elseif action_id == hash("touch") and self.dragging then
        self.runtime:move_player(action.x, action.y)
    end
end

function update(self, dt)
    if not self.dirty then return end
    self.dirty = false
    -- Hook your Defold GUI nodes/factory here. This adapter intentionally does
    -- not create dynamic node IDs from untrusted strings; use a stable ID map.
    -- For each entity in runtime:visible_entities(), read runtime.positions[id]
    -- and set_position() on the corresponding predeclared GUI node.
    -- Render runtime.score, runtime.message and runtime.result into fixed GUI nodes.
end
