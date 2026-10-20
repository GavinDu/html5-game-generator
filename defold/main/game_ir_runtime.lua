-- Canonical declarative Game IR 1.0 interpreter for Defold.
-- Game packages are data only; this module never evaluates downloaded code.
local M = {}

local SUPPORTED_IR = "1.0"
local MAX_ACTIONS_PER_RULE = 32
local MAX_DISPATCH_DEPTH = 16
local ALLOWED_ACTIONS = {
    add_score=true, set_variable=true, move_entity=true, destroy_entity=true,
    spawn_entity=true, play_sound=true, show_text=true, change_scene=true,
    set_health=true, end_game=true
}

local function find_scene(ir, id)
    for _, scene in ipairs(ir.scenes or {}) do
        if scene.id == id then return scene end
    end
end

local function find_entity(scene, id)
    for _, entity in ipairs(scene.entities or {}) do
        if entity.id == id then return entity end
    end
end

local function has_component(entity, name)
    for _, component in ipairs(entity.components or {}) do
        if component == name then return true end
    end
    return false
end

local function safe_number(value, fallback)
    if type(value) == "number" then return value end
    return fallback
end

function M.new(ir)
    assert(type(ir) == "table" and ir.irVersion == SUPPORTED_IR, "unsupported Game IR version")
    assert(type(ir.game) == "table", "Game IR game metadata is missing")
    local scene = find_scene(ir, ir.game.entryScene)
    assert(scene, "Game IR entry scene is missing")

    local state = {
        ir=ir, scene=scene, positions={}, destroyed={}, score=0, health=3,
        variables={}, collected={}, fired_thresholds={}, ended=false, won=false,
        message=nil, dispatch_depth=0
    }

    local emit, execute, score_thresholds

    local function reset_scene(target)
        state.scene = target
        state.positions, state.destroyed, state.collected, state.fired_thresholds = {}, {}, {}, {}
        state.score, state.health, state.ended, state.won, state.message = 0, 3, false, false, nil
        state.variables = {}
        for _, entity in ipairs(target.entities or {}) do
            local p = entity.position or {195, 422}
            state.positions[entity.id] = {safe_number(p[1],195), safe_number(p[2],422)}
        end
    end

    local function conditions_match(conditions)
        for _, condition in ipairs(conditions or {}) do
            local actual, expected = state.variables[condition.variable], condition.value
            if type(actual) ~= "number" or type(expected) ~= "number" then return false end
            local op = condition.operator
            if (op == "eq" and actual ~= expected)
                or (op == "neq" and actual == expected)
                or (op == "gt" and actual <= expected)
                or (op == "gte" and actual < expected)
                or (op == "lt" and actual >= expected)
                or (op == "lte" and actual > expected)
                or not ({eq=true,neq=true,gt=true,gte=true,lt=true,lte=true})[op] then
                return false
            end
        end
        return true
    end

    local function matches(rule, event, source, target, value)
        local when = rule["when"] or {}
        if when.event ~= event then return false end
        if when.source and when.source ~= source then return false end
        if when.target and when.target ~= target then return false end
        if not conditions_match(rule.conditions) then return false end
        if when.value ~= nil then
            if event == "score_reached" then
                if state.score < when.value then return false end
            elseif value == nil or value ~= when.value then
                return false
            end
        end
        return true
    end

    execute = function(actions)
        if type(actions) ~= "table" or #actions > MAX_ACTIONS_PER_RULE then return end
        for _, action in ipairs(actions) do
            if ALLOWED_ACTIONS[action.type] then
                if action.type == "add_score" then
                    state.score = state.score + math.floor(safe_number(action.value, 1))
                    score_thresholds()
                elseif action.type == "set_variable" then
                    if type(action.variable) == "string" and #action.variable <= 64
                        and type(action.value) == "number" then
                        state.variables[action.variable] = action.value
                    end
                elseif action.type == "move_entity" then
                    local p, amount = state.positions[action.target], action.value
                    if p and type(amount) == "number" then p[1] = p[1] + amount end
                elseif action.type == "destroy_entity" then
                    if find_entity(state.scene, action.target) then state.destroyed[action.target] = true end
                elseif action.type == "show_text" then
                    state.message = type(action.value) == "string" and string.sub(action.value, 1, 500) or ""
                elseif action.type == "set_health" then
                    state.health = math.max(0, math.min(999, math.floor(safe_number(action.value, 0))))
                    if state.health == 0 then state.ended, state.won = true, false end
                elseif action.type == "end_game" then
                    state.ended = true
                    state.won = action.value == true or action.value == 1 or action.value == "true"
                elseif action.type == "change_scene" then
                    local target_scene = find_scene(ir, action.sceneId)
                    if target_scene then
                        reset_scene(target_scene)
                        emit("scene_entered", target_scene.id)
                    else
                        state.message = "Unsupported scene target."
                    end
                elseif action.type == "spawn_entity" then
                    -- V1 has no validated entity-template reference; fail closed.
                elseif action.type == "play_sound" then
                    -- Audio playback is a rendering-adapter capability, not executable package code.
                end
            end
            if state.ended then return end
        end
    end

    emit = function(event, source, target, value)
        if state.dispatch_depth >= MAX_DISPATCH_DEPTH then return end
        state.dispatch_depth = state.dispatch_depth + 1
        for _, rule in ipairs(ir.rules or {}) do
            if matches(rule, event, source, target, value) then
                execute(rule.actions or {})
                if state.ended then break end
            end
        end
        state.dispatch_depth = state.dispatch_depth - 1
    end

    score_thresholds = function()
        for _, rule in ipairs(ir.rules or {}) do
            local when = rule["when"] or {}
            if not state.fired_thresholds[rule.id] and matches(rule, "score_reached", "player", nil, state.score) then
                state.fired_thresholds[rule.id] = true
                execute(rule.actions or {})
                if state.ended then return end
            end
        end
    end

    reset_scene(scene)

    function state.emit(event, source, target, value)
        emit(event, source, target, value)
    end

    function state.move_player(x, y)
        if state.ended or type(x) ~= "number" or type(y) ~= "number" then return end
        local player
        for _, entity in ipairs(state.scene.entities or {}) do
            if entity.kind == "player" or has_component(entity, "movement") then player = entity; break end
        end
        if not player then return end
        local size = player.size or {48,48}
        local half_w, half_h = safe_number(size[1],48)/2, safe_number(size[2],48)/2
        local px = math.max(half_w, math.min(390-half_w, x))
        local py = math.max(half_h, math.min(844-half_h, y))
        state.positions[player.id] = {px, py}
        for _, target in ipairs(state.scene.entities or {}) do
            if target.id ~= player.id and not state.destroyed[target.id]
                and (target.kind == "pickup" or has_component(target, "collectible")) then
                local q = state.positions[target.id]
                local target_size = target.size or {32,32}
                if q and math.abs(px-q[1]) < (safe_number(size[1],48)+safe_number(target_size[1],32))*0.45
                    and math.abs(py-q[2]) < (safe_number(size[2],48)+safe_number(target_size[2],32))*0.45 then
                    local key = player.id .. ":" .. target.id
                    if not state.collected[key] then
                        state.collected[key] = true
                        emit("collected", player.id, target.id)
                    end
                end
            end
        end
    end

    return state
end

return M
