// Pure bounded semantic state reducer candidate for Game IR v2.
// No renderer, physics, audio, browser, storage, network, or wall time.
// This module is NOT yet wired into the production Pixi interpreter.
export const SEMANTIC_STATE_SCHEMA = 'semantic-state-v1';
export const MAX_SEMANTIC_STATE_TICKS = 108000;
export const MAX_SEMANTIC_ACTIONS_PER_TICK = 256;
export const MAX_SEMANTIC_VARIABLES = 128;
const MAX_ABS_NUMBER = 1_000_000_000;
const MAX_KEY_BYTES = 128;
const MAX_STRING_BYTES = 256;
const encoder = new TextEncoder();
const hasOwn = (value, key) => Object.prototype.hasOwnProperty.call(value, key);

function isPlainObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value) &&
    (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
}

function isFiniteBounded(value) {
  return typeof value === 'number' && Number.isFinite(value) &&
    Math.abs(value) <= MAX_ABS_NUMBER;
}

function isIntegerBounded(value) {
  return isFiniteBounded(value) && Number.isSafeInteger(value);
}

function wellFormedUTF16(s) {
  for (let i = 0; i < s.length; ++i) {
    const code = s.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      if (++i === s.length) return false;
      const low = s.charCodeAt(i);
      if (low < 0xdc00 || low > 0xdfff) return false;
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      return false;
    }
  }
  return true;
}

function validVariableKey(key) {
  return typeof key === 'string' && key.length !== 0 &&
    key !== '__proto__' && key !== 'constructor' && key !== 'prototype' &&
    wellFormedUTF16(key) && encoder.encode(key).length <= MAX_KEY_BYTES;
}

function validPrimitive(value) {
  return isFiniteBounded(value) || typeof value === 'boolean' ||
    (typeof value === 'string' && wellFormedUTF16(value) &&
      encoder.encode(value).length <= MAX_STRING_BYTES);
}

function validateVariables(raw) {
  if (!isPlainObject(raw)) throw new TypeError('semantic variables must be plain data');
  const keys = Object.keys(raw);
  if (keys.length > MAX_SEMANTIC_VARIABLES) throw new RangeError('semantic variable budget exceeded');
  const variables = Object.create(null);
  for (const key of keys.sort()) {
    if (!validVariableKey(key) || !validPrimitive(raw[key])) {
      throw new TypeError('invalid semantic variable');
    }
    variables[key] = raw[key];
  }
  return Object.freeze(variables);
}

function validTerminal(value) {
  return value === null || value === 'win' || value === 'lose';
}

function validateState(raw) {
  if (!isPlainObject(raw) || raw.schema !== SEMANTIC_STATE_SCHEMA ||
      !Number.isSafeInteger(raw.tick) || raw.tick < 0 ||
      raw.tick > MAX_SEMANTIC_STATE_TICKS ||
      !Number.isSafeInteger(raw.seed) || raw.seed < 0 || raw.seed > 0xffffffff ||
      !isIntegerBounded(raw.score) || !isIntegerBounded(raw.health) || raw.health < 0 ||
      !validTerminal(raw.terminal)) {
    throw new TypeError('invalid semantic state');
  }
  const keys = Object.keys(raw);
  if (keys.length !== 7 ||
      keys.some(k => !['schema', 'tick', 'seed', 'score', 'health', 'variables', 'terminal'].includes(k))) {
    throw new TypeError('unknown semantic state field');
  }
  return Object.freeze({
    schema: SEMANTIC_STATE_SCHEMA,
    tick: raw.tick,
    seed: raw.seed,
    score: raw.score,
    health: raw.health,
    variables: validateVariables(raw.variables),
    terminal: raw.terminal,
  });
}

export function createSemanticState({ seed = 0, score = 0, health = 100, variables = {} } = {}) {
  return validateState({
    schema: SEMANTIC_STATE_SCHEMA,
    tick: 0,
    seed,
    score,
    health,
    variables,
    terminal: null,
  });
}

// A canonical *string representation*, not a cryptographic attestation.
// JSON property insertion order is never relied upon for variable keys.
export function canonicalSemanticState(state) {
  const value = validateState(state);
  return JSON.stringify([
    value.schema, value.tick, value.seed, value.score, value.health,
    Object.keys(value.variables).sort().map(k => [k, value.variables[k]]),
    value.terminal,
  ]);
}

function validateAction(action) {
  if (!isPlainObject(action) || typeof action.type !== 'string') {
    throw new TypeError('invalid semantic action');
  }
  const type = action.type;
  const keys = Object.keys(action);
  const simple = type === 'add_score' || type === 'set_health' ||
    type === 'change_health' || type === 'end_game';
  const variable = type === 'set_variable' || type === 'add_variable';
  if ((!simple && !variable) || !hasOwn(action, 'value') ||
      keys.length !== (simple ? 2 : 3) ||
      keys.some(k => !['type', 'value', ...(variable ? ['variable'] : [])].includes(k))) {
    throw new TypeError('unknown semantic action or property');
  }
  if (variable && !validVariableKey(action.variable)) {
    throw new TypeError('invalid semantic variable key');
  }
  if (type === 'end_game') {
    if (typeof action.value !== 'boolean') throw new TypeError('end_game must be boolean');
  } else if (type === 'set_variable') {
    if (!validPrimitive(action.value)) throw new TypeError('invalid semantic variable value');
  } else if (!isIntegerBounded(action.value)) {
    throw new TypeError('numeric semantic action must be a bounded integer');
  }
}

// Input order *is* the authoritative same-tick order: zero-based contiguous
// ordinals, no sorting or hidden timestamp normalization in this reducer.
export function reduceSemanticStateTick(previous, tick, orderedCommands) {
  const state = validateState(previous);
  if (!Array.isArray(orderedCommands) ||
      orderedCommands.length > MAX_SEMANTIC_ACTIONS_PER_TICK) {
    throw new RangeError('semantic command budget exceeded');
  }
  if (state.terminal !== null) {
    if (orderedCommands.length !== 0) throw new TypeError('post-terminal actions rejected');
    return { state, effects: Object.freeze([]) };
  }
  if (!Number.isSafeInteger(tick) || tick !== state.tick + 1 ||
      tick > MAX_SEMANTIC_STATE_TICKS) {
    throw new RangeError('semantic tick must advance exactly once');
  }
  for (let i = 0; i < orderedCommands.length; i++) {
    const item = orderedCommands[i];
    if (!isPlainObject(item) || Object.keys(item).length !== 2 ||
        !hasOwn(item, 'ordinal') || !hasOwn(item, 'action') ||
        !Number.isSafeInteger(item.ordinal) || item.ordinal !== i) {
      throw new TypeError('semantic commands require exact zero-based FIFO ordinals');
    }
    validateAction(item.action);
  }

  let score = state.score;
  let health = state.health;
  let terminal = null;
  const variables = Object.assign(Object.create(null), state.variables);
  const effects = [];

  for (const { action } of orderedCommands) {
    if (terminal !== null) break;
    const beforeScore = score;
    const beforeHealth = health;
    switch (action.type) {
      case 'add_score':
        score += action.value;
        if (!isIntegerBounded(score)) throw new RangeError('score overflow');
        effects.push(Object.freeze({ type: 'score_changed', value: score, previous: beforeScore }));
        break;
      case 'set_variable':
        if (!hasOwn(variables, action.variable) &&
            Object.keys(variables).length >= MAX_SEMANTIC_VARIABLES) {
          throw new RangeError('semantic variable budget exceeded');
        }
        variables[action.variable] = action.value;
        effects.push(Object.freeze({ type: 'variable_changed', key: action.variable }));
        break;
      case 'add_variable': {
        const before = hasOwn(variables, action.variable) ? variables[action.variable] : 0;
        if (!isFiniteBounded(before)) throw new TypeError('add_variable requires numeric state');
        const updated = before + action.value;
        if (!isFiniteBounded(updated)) throw new RangeError('variable overflow');
        if (!hasOwn(variables, action.variable) &&
            Object.keys(variables).length >= MAX_SEMANTIC_VARIABLES) {
          throw new RangeError('semantic variable budget exceeded');
        }
        variables[action.variable] = updated;
        effects.push(Object.freeze({ type: 'variable_changed', key: action.variable }));
        break;
      }
      case 'set_health':
        health = Math.max(0, action.value);
        effects.push(Object.freeze({ type: 'health_changed', value: health, previous: beforeHealth }));
        if (health === 0) terminal = 'lose';
        break;
      case 'change_health':
        health = Math.max(0, health + action.value);
        if (!isIntegerBounded(health)) throw new RangeError('health overflow');
        effects.push(Object.freeze({ type: 'health_changed', value: health, previous: beforeHealth }));
        if (health === 0) terminal = 'lose';
        break;
      case 'end_game':
        terminal = action.value ? 'win' : 'lose';
        break;
    }
    if (terminal !== null) effects.push(Object.freeze({ type: 'terminal', result: terminal }));
  }
  const next = validateState({
    schema: SEMANTIC_STATE_SCHEMA,
    tick,
    seed: state.seed,
    score,
    health,
    variables,
    terminal,
  });
  return Object.freeze({ state: next, effects: Object.freeze(effects) });
}
