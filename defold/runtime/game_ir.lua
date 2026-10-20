-- Game IR 1.0 interpreter for Defold.
-- This module interprets declarative data only; it never loads/evaluates downloaded Lua.
local M = {}
local Runtime = {}
Runtime.__index = Runtime

local SUPPORTED_IR = "1.0"
local MAX_RULE_ACTIONS = 32
local ALLOWED_ACTIONS = {
    add_score=true, set_variable=true, move_entity=true, destroy_entity=true,
    spawn_entity=true, play_sound=true, show_text=true, change_scene=true,
    set_health=true, end_game=true
}

local function copy_table(value)
    if type(value) ~= "table" then return value end
    local out = {}
    for k, v in pairs(value) do out[k] = copy_table(v) end
    return out
end

local function number(value, fallback)
    if type(value) == "number" then return value end
    return fallback
end

local function find_scene(doc, id)
    for _, scene in ipairs(doc.scenes or {}) do
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

function M.new(document, callbacks)
    assert(type(document) == "table", "Game IR document must be a table")
    assert(document.irVersion == SUPPORTED_IR, "unsupported Game IR version: " .. tostring(document.irVersion))
    assert(type(document.game) == "table", "Game IR game is missing")
    local scene = find_scene(document, document.game.entryScene)
    assert(scene, "entry scene is missing: " .. tostring(document.game.entryScene))
    callbacks = callbacks or {}
    local self = setmetatable({
        document = document,
        callbacks = callbacks,
        scene = scene,
        positions = {},
        destroyed = {},
        variables = {},
        collected = {},
        fired_thresholds = {},
        score = 0,
        health = 3,
        ended = false,
        won = false,
        message = nil,
        _dispatch_depth = 0
    }, Runtime)
    self:reset(false)
    self:emit("game_start")
    return self
end

function Runtime:reset(run_start)
    self.positions, self.destroyed, self.variables, self.collected, self.fired_thresholds = {}, {}, {}, {}, {}
    self.score, self.health, self.ended, self.won, self.message = 0, 3, false, false, nil
    for _, entity in ipairs(self.scene.entities or {}) do
        self.positions[entity.id] = {
            x = number(entity.position and entity.position[1], 195),
            y = number(entity.position and entity.position[2], 422)
        }
    end
    self:_notify("reset")
    if run_start then self:emit("game_start") end
end

function Runtime:_notify(event, payload)
    local fn = self.callbacks[event]
    if fn then fn(payload or self) end
end

function Runtime:visible_entities()
    local result = {}
    for _, entity in ipairs(self.scene.entities or {}) do
        if not self.destroyed[entity.id] then result[#result + 1] = entity end
    end
    return result
end

local function conditions_match(self, conditions)
    for _, condition in ipairs(conditions or {}) do
        local actual, expected = self.variables[condition.variable], condition.value
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

local function rule_matches(self, rule, event, source, target, value)
    local when = rule["when"] or {}
    if when.event ~= event then return false end
    if when.source and when.source ~= source then return false end
    if when.target and when.target ~= target then return false end
    if not conditions_match(self, rule.conditions) then return false end
    if when.value ~= nil then
        if event == "score_reached" then
            if self.score < when.value then return false end
        elseif value == nil or value ~= when.value then
            return false
        end
    end
    return true
end

function Runtime:emit(event, source, target, value)
    -- Bound recursion prevents malformed rule chains from freezing the frame loop.
    if self._dispatch_depth >= 16 then return end
    self._dispatch_depth = self._dispatch_depth + 1
    for _, rule in ipairs(self.document.rules or {}) do
        if rule_matches(self, rule, event, source, target, value) then
            self:execute(rule.actions or {})
            if self.ended then break end
        end
    end
    self._dispatch_depth = self._dispatch_depth - 1
    self:_notify("state_changed", {event=event, source=source, target=target, value=value})
end

function Runtime:_score_thresholds()
    for _, rule in ipairs(self.document.rules or {}) do
        local when = rule["when"] or {}
        if not self.fired_thresholds[rule.id]
            and rule_matches(self, rule, "score_reached", "player", nil, self.score) then
            self.fired_thresholds[rule.id] = true
            self:execute(rule.actions or {})
            if self.ended then return end
        end
    end
end

function Runtime:execute(actions)
    if type(actions) ~= "table" or #actions > MAX_RULE_ACTIONS then return end
    for _, action in ipairs(actions) do
        if not ALLOWED_ACTIONS[action.type] then
            -- Unknown actions are ignored. No code or dynamic module is evaluated.
        elseif action.type == "add_score" then
            self.score = self.score + math.floor(number(action.value, 1))
            self:_notify("score_changed", self.score)
            self:_score_thresholds()
        elseif action.type == "set_variable" then
            if type(action.variable) == "string" and #action.variable <= 64 then
                local value = action.value
                if type(value) == "number" then self.variables[action.variable] = value end
            end
        elseif action.type == "move_entity" then
            local pos = self.positions[action.target]
            if pos and type(action.value) == "number" then
                pos.x = pos.x + action.value
            end
        elseif action.type == "destroy_entity" then
            if find_entity(self.scene, action.target) then
                self.destroyed[action.target] = true
                self:_notify("entity_destroyed", action.target)
            end
        elseif action.type == "show_text" then
            self.message = type(action.value) == "string" and string.sub(action.value, 1, 500) or ""
            self:_notify("show_text", self.message)
        elseif action.type == "set_health" then
            self.health = math.max(0, math.min(999, math.floor(number(action.value, 0))))
            self:_notify("health_changed", self.health)
            if self.health == 0 then self.ended, self.won = true, false end
        elseif action.type == "end_game" then
            self.ended = true
            self.won = action.value == true or action.value == 1 or action.value == "true"
            self:_notify("game_ended", {won=self.won, score=self.score})
        elseif action.type == "change_scene" then
            local next_scene = find_scene(self.document, action.sceneId)
            if next_scene then
                self.scene = next_scene
                self:reset(false)
                self:emit("scene_entered", next_scene.id)
            end
        elseif action.type == "play_sound" then
            if type(self.callbacks.play_sound) == "function" then
                self.callbacks.play_sound(action.assetId)
            end
        elseif action.type == "spawn_entity" then
            -- Spawn requires a bounded, validated entity template in a future IR revision.
            -- Do not construct arbitrary entities from untrusted rule payloads.
        end
        if self.ended then break end
    end
end

function Runtime:move_player(x, y)
    if self.ended or type(x) ~= "number" or type(y) ~= "number" then return end
    local player
    for _, entity in ipairs(self.scene.entities or {}) do
        if entity.kind == "player" or has_component(entity, "movement") then player = entity; break end
    end
    if not player then return end
    local size = player.size or {48, 48}
    local half_w, half_h = number(size[1], 48)/2, number(size[2], 48)/2
    self.positions[player.id] = {
        x=math.max(half_w, math.min(390-half_w, x)),
        y=math.max(half_h, math.min(844-half_h, y))
    }
    self:check_collections(player)
    self:_notify("state_changed", {event="move", entity=player.id})
end

function Runtime:check_collections(player)
    local p = self.positions[player.id]
    if not p then return end
    for _, target in ipairs(self:visible_entities()) do
        if target.id ~= player.id and (target.kind == "pickup" or has_component(target, "collectible")) then
            local q = self.positions[target.id]
            if q then
                local ps, ts = player.size or {48,48}, target.size or {48,48}
                if math.abs(p.x-q.x) < (number(ps[1],48)+number(ts[1],48))*0.45
                    and math.abs(p.y-q.y) < (number(ps[2],48)+number(ts[2],48))*0.45 then
                    local key = player.id .. ":" .. target.id
                    if not self.collected[key] then
                        self.collected[key] = true
                        self:emit("collected", player.id, target.id)
                    end
                end
            end
        end
    end
end

function Runtime:input(action, value)
    if self.ended then return end
    for _, binding in ipairs(self.document.inputBindings or {}) do
        if binding.action == action and binding.input == value then
            self:emit("input", nil, nil, nil)
            return true
        end
    end
    return false
end

function Runtime:snapshot()
    return {
        gameId=self.document.game.id, sceneId=self.scene.id,
        positions=copy_table(self.positions), destroyed=copy_table(self.destroyed),
        score=self.score, health=self.health, ended=self.ended, won=self.won,
        message=self.message
    }
end

M.Runtime = Runtime
M.version = SUPPORTED_IR
return M
