import test from 'node:test';
import assert from 'node:assert/strict';
import {
  SEMANTIC_STATE_SCHEMA,
  MAX_SEMANTIC_STATE_TICKS,
  MAX_SEMANTIC_ACTIONS_PER_TICK,
  MAX_SEMANTIC_VARIABLES,
  createSemanticState,
  canonicalSemanticState,
  reduceSemanticStateTick,
} from '../../internal/api/web/runtime-semantic-state-reducer.mjs';

const cmd = (ordinal, type, value, variable) => ({
  ordinal,
  action: {
    type, value,
    ...(variable === undefined ? {} : { variable }),
  },
});

test('pure semantic state transitions use FIFO action order and immutable typed effects', () => {
  const initial = createSemanticState({
    seed: 7, score: 1, health: 8, variables: { wins: 2, mode: 'rainbow' },
  });
  assert.equal(initial.schema, SEMANTIC_STATE_SCHEMA);
  const commands = [
    cmd(0, 'add_score', 4),
    cmd(1, 'set_variable', 5, 'wins'),
    cmd(2, 'add_variable', -3, 'wins'),
    cmd(3, 'change_health', -2),
  ];
  const result = reduceSemanticStateTick(initial, 1, commands);
  assert.deepEqual({
    tick: result.state.tick, seed: result.state.seed, score: result.state.score,
    health: result.state.health, wins: result.state.variables.wins,
    mode: result.state.variables.mode, terminal: result.state.terminal,
  }, { tick: 1, seed: 7, score: 5, health: 6, wins: 2, mode: 'rainbow', terminal: null });
  assert.deepEqual(result.effects.map(e => e.type), [
    'score_changed', 'variable_changed', 'variable_changed', 'health_changed',
  ]);
  assert.deepEqual(result.effects[0], { type: 'score_changed', previous: 1, value: 5 });
  assert.deepEqual(result.effects[3], { type: 'health_changed', previous: 8, value: 6 });
  assert.equal(initial.score, 1);
  assert.equal(initial.health, 8);
  assert.equal(initial.variables.wins, 2);
  assert.equal(initial.tick, 0);
  assert.ok(Object.isFrozen(initial));
  assert.ok(Object.isFrozen(result));
  assert.ok(Object.isFrozen(result.state));
  assert.ok(Object.isFrozen(result.effects));
  assert.ok(Object.isFrozen(result.effects[0]));
  assert.ok(Object.isFrozen(result.state.variables));
  assert.throws(() => { result.state.variables.wins = 9; }, TypeError);
  assert.throws(() => { result.effects.push({ type: 'fake' }); }, TypeError);
});

test('canonical snapshots are stable across variable insertion order and quality/pacing', () => {
  const first = createSemanticState({ seed: 99, variables: { z: 'Z', a: 1, b: true } });
  const second = createSemanticState({ seed: 99, variables: { b: true, z: 'Z', a: 1 } });
  assert.equal(canonicalSemanticState(first), canonicalSemanticState(second));
  const commands = [cmd(0, 'add_score', 2), cmd(1, 'set_variable', 'normal', 'qualityLabel')];
  const highQuality = reduceSemanticStateTick(first, 1, commands).state;
  const lowQuality = reduceSemanticStateTick(second, 1, commands).state;
  assert.equal(canonicalSemanticState(highQuality), canonicalSemanticState(lowQuality));
  assert.notEqual(canonicalSemanticState(first), canonicalSemanticState(highQuality));
  assert.equal(canonicalSemanticState(highQuality), canonicalSemanticState(highQuality));
  const parsed = JSON.parse(canonicalSemanticState(highQuality));
  assert.deepEqual(parsed[5].map(([key]) => key), ['a', 'b', 'qualityLabel', 'z']);
  // There is no presentation-quality argument or wall-clock input.
  const empty = reduceSemanticStateTick(highQuality, 2, []);
  assert.equal(empty.state.tick, 2);
  assert.deepEqual(empty.effects, []);
  assert.equal(empty.state.score, 2);
});

test('health exhaustion ends the tick and all subsequent commands are ignored safely', () => {
  const initial = createSemanticState({ seed: 1, health: 2 });
  const first = reduceSemanticStateTick(initial, 1, [
    cmd(0, 'change_health', -5), cmd(1, 'add_score', 900),
  ]);
  assert.equal(first.state.health, 0);
  assert.equal(first.state.terminal, 'lose');
  assert.equal(first.state.score, 0);
  assert.deepEqual(first.effects.map(e => e.type), ['health_changed', 'terminal']);
  const after = reduceSemanticStateTick(first.state, 2, []);
  assert.deepEqual(after.state, first.state);
  assert.equal(canonicalSemanticState(after.state), canonicalSemanticState(first.state));
  assert.deepEqual(after.effects, []);
  assert.throws(() => reduceSemanticStateTick(first.state, 2, [
    cmd(0, 'add_score', 1),
  ]), /post-terminal/);
  const explicit = reduceSemanticStateTick(createSemanticState({ seed: 2 }), 1, [
    cmd(0, 'add_score', 3), cmd(1, 'end_game', true), cmd(2, 'add_score', 9),
  ]);
  assert.equal(explicit.state.score, 3);
  assert.equal(explicit.state.terminal, 'win');
  assert.deepEqual(explicit.effects.map(e => e.type), ['score_changed', 'terminal']);
  assert.equal(initial.health, 2);
});

test('invalid tick, FIFO order, action schema and side-effect injection all fail closed', () => {
  const initial = createSemanticState({ seed: 23 });
  const badCommands = [
    [cmd(1, 'add_score', 1)],
    [cmd(0, 'add_score', 1), cmd(0, 'add_score', 2)],
    [cmd(0, 'add_score', 1), cmd(2, 'add_score', 2)],
    [{ ordinal: 0, action: { type: 'add_score', value: 1, hacked: true } }],
    [{ ordinal: 0, action: { type: 'render', value: 1 } }],
    [{ ordinal: 0, action: { type: 'add_score', value: 1 }, extra: 'execute' }],
    [{ ordinal: -1, action: { type: 'add_score', value: 1 } }],
    [{ ordinal: 0, action: { type: 'add_score', value: NaN } }],
    [{ ordinal: 0, action: { type: 'add_score', value: Infinity } }],
    [{ ordinal: 0, action: { type: 'add_score', value: 1.5 } }],
    [{ ordinal: 0, action: { type: 'set_variable', value: 1, variable: '__proto__' } }],
    [{ ordinal: 0, action: { type: 'set_variable', value: 1, variable: 'constructor' } }],
    [{ ordinal: 0, action: { type: 'set_variable', value: 1, variable: 'prototype' } }],
    [{ ordinal: 0, action: { type: 'set_variable', value: 1, variable: '\uD800' } }],
    [{ ordinal: 0, action: { type: 'set_variable', value: {}, variable: 'safe' } }],
    [{ ordinal: 0, action: { type: 'end_game', value: 1 } }],
  ];
  for (const bad of badCommands) {
    assert.throws(() => reduceSemanticStateTick(initial, 1, bad), undefined,
      'must reject hostile input: ' + JSON.stringify(bad));
    assert.equal(initial.score, 0, 'failure must not mutate previous state');
  }
  assert.throws(() => reduceSemanticStateTick(initial, 0, []), /tick/);
  assert.throws(() => reduceSemanticStateTick(initial, 2, []), /tick/);
  assert.throws(() => reduceSemanticStateTick(initial, 1, new Array(MAX_SEMANTIC_ACTIONS_PER_TICK + 1).fill(cmd(0, 'add_score', 1))), /budget/);
  assert.throws(() => reduceSemanticStateTick(initial, 1, null), /budget/);
});

test('input validation rejects unbounded numeric, text, variables and malformed state', () => {
  assert.equal(MAX_SEMANTIC_STATE_TICKS, 108000);
  assert.equal(MAX_SEMANTIC_VARIABLES, 128);
  assert.throws(() => createSemanticState({ seed: -1 }), /state/);
  assert.throws(() => createSemanticState({ seed: 0x1_0000_0000 }), /state/);
  assert.throws(() => createSemanticState({ health: -1 }), /state/);
  assert.throws(() => createSemanticState({ score: Number.MAX_SAFE_INTEGER }), /state/);
  assert.throws(() => createSemanticState({ variables: { bad: NaN } }), /variable/);
  assert.throws(() => createSemanticState({ variables: { bad: '\uD800' } }), /variable/);
  assert.throws(() => createSemanticState({ variables: { bad: 'x'.repeat(257) } }), /variable/);
  assert.throws(() => createSemanticState({ variables: { ['é'.repeat(65)]: 1 } }), /variable/);
  assert.throws(() => createSemanticState({ variables: Object.fromEntries(
    Array.from({ length: MAX_SEMANTIC_VARIABLES + 1 }, (_, i) => ['v' + i, i]),
  ) }), /budget/);
  const state = createSemanticState({ score: 1_000_000_000 });
  assert.throws(() => reduceSemanticStateTick(state, 1, [cmd(0, 'add_score', 1)]), /overflow/);
  const variable = createSemanticState({ variables: { answer: true } });
  assert.throws(() => reduceSemanticStateTick(variable, 1, [cmd(0, 'add_variable', 1, 'answer')]), /numeric/);
  const atCap = createSemanticState({ variables: Object.fromEntries(
    Array.from({ length: MAX_SEMANTIC_VARIABLES }, (_, i) => ['v' + i, i]),
  ) });
  assert.throws(() => reduceSemanticStateTick(atCap, 1, [cmd(0, 'set_variable', 1, 'overflow')]), /budget/);
  const end = reduceSemanticStateTick(createSemanticState(), 1, [cmd(0, 'end_game', false)]);
  assert.throws(() => canonicalSemanticState({ ...end.state, unexpected: 'script' }), /unknown/);
});
