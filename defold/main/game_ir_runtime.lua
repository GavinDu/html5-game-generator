-- Declarative Game IR 1.0 interpreter. Never executes package-provided code.
local M = {}
local function copy_position(p) return {p[1] or 195, p[2] or 422} end

function M.new(ir)
    assert(type(ir) == "table" and ir.irVersion == "1.0", "unsupported Game IR")
    local scene
    for _, candidate in ipairs(ir.scenes or {}) do
        if candidate.id == ir.game.entryScene then scene = candidate break end
    end
    assert(scene, "entry scene missing")
    local state = {ir=ir, scene=scene, positions={}, destroyed={}, score=0, health=3, variables={}, collected={}, ended=false, won=false, message=nil}
    for _, entity in ipairs(scene.entities or {}) do state.positions[entity.id] = copy_position(entity.position or {195,422}) end

    local emit
    local function execute(actions)
        for _, action in ipairs(actions or {}) do
            if action.type == "add_score" then
                state.score = state.score + (tonumber(action.value) or 1)
                emit("score_reached", nil, nil, state.score)
            elseif action.type == "set_variable" then
                if action.variable then state.variables[action.variable] = action.value end
            elseif action.type == "move_entity" then
                local p, amount = state.positions[action.target], tonumber(action.value)
                if p and amount then p[1] = p[1] + amount end
            elseif action.type == "destroy_entity" then
                if action.target then state.destroyed[action.target] = true end
            elseif action.type == "show_text" then
                state.message = tostring(action.value or "")
            elseif action.type == "set_health" then
                state.health = tonumber(action.value) or 0
                if state.health <= 0 then state.ended = true; state.won = false end
            elseif action.type == "end_game" then
                state.ended = true
                state.won = action.value ~= false
            elseif action.type == "change_scene" then
                state.message = "Scene changes are not supported in this runtime slice."
            elseif action.type == "spawn_entity" or action.type == "play_sound" then
                state.message = "Not yet supported: " .. action.type
            end
        end
    end

    emit = function(event, source, target, value)
        for _, rule in ipairs(ir.rules or {}) do
            local when = rule["when"] or {}
            local value_matches = when.value == nil or when.value == value or (event == "score_reached" and value ~= nil and value >= when.value)
            if when.event == event and (when.source == nil or when.source == source) and (when.target == nil or when.target == target) and value_matches then
                execute(rule.actions)
            end
        end
    end
    function state.emit(event, source, target, value) emit(event, source, target, value) end

    function state.move_player(x, y)
        if state.ended then return end
        local player
        for _, entity in ipairs(scene.entities or {}) do
            local has_movement = false
            for _, component in ipairs(entity.components or {}) do if component == "movement" then has_movement = true end end
            if entity.kind == "player" or has_movement then player = entity break end
        end
        if not player then return end
        local size = player.size or {48,48}
        local px, py = math.max(size[1]/2, math.min(390-size[1]/2, x)), math.max(size[2]/2, math.min(844-size[2]/2, y))
        state.positions[player.id] = {px,py}
        for _, target_entity in ipairs(scene.entities or {}) do
            local id, collectible = target_entity.id, target_entity.kind == "pickup"
            for _, component in ipairs(target_entity.components or {}) do if component == "collectible" then collectible = true end end
            if collectible and id ~= player.id and not state.destroyed[id] then
                local target, target_size = state.positions[id] or target_entity.position, target_entity.size or {32,32}
                if target and math.abs(px-target[1]) < (size[1]+target_size[1])*0.45 and math.abs(py-target[2]) < (size[2]+target_size[2])*0.45 then
                    local key = player.id .. ":" .. id
                    if not state.collected[key] then state.collected[key] = true; emit("collected", player.id, id) end
                end
            end
        end
    end
    return state
end
return M
