import test from 'node:test';
import assert from 'node:assert/strict';
import {
  createSemanticInputV2Recorder,
  SEMANTIC_INPUT_V2_SCHEMA,
  MAX_SEMANTIC_INPUT_V2_EVENTS,
  MAX_SEMANTIC_INPUT_V2_TICKS,
} from '../../internal/api/web/runtime-semantic-input-v2.mjs';

const bindings = [
  { action: 'jump', input: 'tap' },
  { action: 'aim', input: 'drag' },
  { action: 'move-left', input: 'keyboard', key: 'ArrowLeft' },
];

const fixed = (x, y) => ({ x, y });

test('v2 records declared same-tick edge ordinals and explicit cancel with fixed coordinates', () => {
  const writer = createSemanticInputV2Recorder(bindings);
  assert.equal(writer.record({ tick: 2, action: 'aim', device: 'pointer', phase: 'down', position: fixed(1000, -2000) }), true);
  assert.equal(writer.record({ tick: 2, action: 'aim', device: 'pointer', phase: 'move', position: fixed(3000, -4000) }), true);
  assert.equal(writer.record({ tick: 2, action: 'move-left', device: 'keyboard', phase: 'down' }), true);
  assert.equal(writer.record({ tick: 3, action: 'aim', device: 'pointer', phase: 'cancel' }), true);
  assert.equal(writer.record({ tick: 3, action: 'move-left', device: 'keyboard', phase: 'up' }), true);
  assert.equal(writer.eventCount, 5);
  const result = writer.finish(10);
  assert.deepEqual(result, {
    schema: SEMANTIC_INPUT_V2_SCHEMA,
    durationTicks: 10,
    events: [
      { tick: 2, ordinal: 0, action: 'aim', device: 'pointer', phase: 'down', position: fixed(1000, -2000) },
      { tick: 2, ordinal: 1, action: 'aim', device: 'pointer', phase: 'move', position: fixed(3000, -4000) },
      { tick: 2, ordinal: 2, action: 'move-left', device: 'keyboard', phase: 'down' },
      { tick: 3, ordinal: 0, action: 'aim', device: 'pointer', phase: 'cancel' },
      { tick: 3, ordinal: 1, action: 'move-left', device: 'keyboard', phase: 'up' },
    ],
  });
  assert.equal(writer.complete, true);
  assert.equal(writer.finished, true);
  assert.equal(writer.record({ tick: 4, action: 'jump', device: 'pointer', phase: 'down', position: fixed(1, 2) }), false);
  assert.equal(writer.finish(10), null, 'a trace may only finalize once');
  result.events[0].position.x = 999;
  assert.equal(writer.eventCount, 5, 'returning a wire value must not mutate recorder state');
});

test('empty v2 trace is valid but an unfinished contact cannot be silently released', () => {
  const idle = createSemanticInputV2Recorder(bindings);
  assert.deepEqual(idle.finish(1), { schema: SEMANTIC_INPUT_V2_SCHEMA, durationTicks: 1, events: [] });
  const held = createSemanticInputV2Recorder(bindings);
  held.record({ tick: 0, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0) });
  assert.equal(held.finish(1), null);
  assert.equal(held.failureReason, 'incomplete_trace');
  assert.equal(held.record({ tick: 1, action: 'jump', device: 'pointer', phase: 'up' }), false);
  assert.equal(held.eventCount, 1, 'no fake cancel or release may be invented');
  const next = createSemanticInputV2Recorder(bindings);
  assert.equal(next.eventCount, 0);
  assert.equal(next.complete, true);
});

test('v2 wrong action, tap move, duplicate down, backwards tick, release and position all fail closed', () => {
  const cases = [
    [[{ tick: 1, action: 'cheat', device: 'pointer', phase: 'down', position: fixed(0, 0) }], 'undeclared_action'],
    [[{ tick: 1, action: 'jump', device: 'pointer', phase: 'move', position: fixed(0, 0) }], 'invalid_move'],
    [[{ tick: 1, action: 'jump', device: 'pointer', phase: 'up' }], 'unbalanced_release'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: fixed(0, 0) },
      { tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: fixed(0, 0) }], 'duplicate_down'],
    [[{ tick: 2, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0) },
      { tick: 1, action: 'jump', device: 'pointer', phase: 'up' }], 'backwards_tick'],
    [[{ tick: 1, action: 'move-left', device: 'keyboard', phase: 'down', position: fixed(0, 0) }], 'unexpected_position'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down' }], 'invalid_position'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: fixed(1.2, 0) }], 'invalid_position'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: fixed(1_000_000_001, 0) }], 'invalid_position'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: { x: 0, y: 0, z: 0 } }], 'invalid_position'],
    [[{ tick: 1, action: 'aim', device: 'pointer', phase: 'down', position: fixed(0, 0), verified: true }], 'invalid_event'],
    [[{ tick: -1, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0) }], 'invalid_event'],
    [[{ tick: 1.5, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0) }], 'invalid_event'],
  ];
  for (const [edges, reason] of cases) {
    const writer = createSemanticInputV2Recorder(bindings);
    for (const edge of edges) writer.record(edge);
    assert.equal(writer.complete, false, JSON.stringify(edges));
    assert.equal(writer.failureReason, reason);
    assert.equal(writer.finish(10), null);
    const before = writer.eventCount;
    assert.equal(writer.record({ tick: 10, action: 'jump', device: 'pointer', phase: 'up' }), false);
    assert.equal(writer.eventCount, before);
  }
});

test('v2 event boundary retains complete prefix, never overwrites oldest, and caps duration', () => {
  assert.equal(MAX_SEMANTIC_INPUT_V2_EVENTS, 2000);
  assert.equal(MAX_SEMANTIC_INPUT_V2_TICKS, 108000);
  const writer = createSemanticInputV2Recorder(bindings);
  for (let i = 0; i < MAX_SEMANTIC_INPUT_V2_EVENTS; i++) {
    assert.equal(writer.record({
      tick: i, action: 'jump', device: 'pointer', phase: i % 2 === 0 ? 'down' : 'up',
      ...(i % 2 === 0 ? { position: fixed(i, i) } : {}),
    }), true);
  }
  assert.equal(writer.eventCount, 2000);
  assert.equal(writer.record({
    tick: 2000, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0),
  }), false);
  assert.equal(writer.failureReason, 'event_limit');
  assert.equal(writer.eventCount, 2000);
  assert.equal(writer.finish(2000), null);

  const atEdge = createSemanticInputV2Recorder(bindings);
  assert.equal(atEdge.record({
    tick: MAX_SEMANTIC_INPUT_V2_TICKS, action: 'jump', device: 'pointer',
    phase: 'down', position: fixed(-1_000_000_000, 1_000_000_000),
  }), true);
  assert.equal(atEdge.record({
    tick: MAX_SEMANTIC_INPUT_V2_TICKS, action: 'jump', device: 'pointer', phase: 'up',
  }), true);
  assert.equal(atEdge.finish(MAX_SEMANTIC_INPUT_V2_TICKS)?.events.length, 2);
  const incomplete = createSemanticInputV2Recorder(bindings);
  incomplete.record({ tick: 20, action: 'jump', device: 'pointer', phase: 'down', position: fixed(0, 0) });
  incomplete.record({ tick: 21, action: 'jump', device: 'pointer', phase: 'up' });
  assert.equal(incomplete.finish(20), null, 'declared duration cannot omit the last action tick');
});

test('v2 constructor rejects ambiguous bindings, invalid action/byte length and budgets', () => {
  assert.throws(() => createSemanticInputV2Recorder([]), RangeError);
  assert.throws(() => createSemanticInputV2Recorder(bindings, 0), RangeError);
  assert.throws(() => createSemanticInputV2Recorder(bindings, 2001), RangeError);
  assert.throws(() => createSemanticInputV2Recorder(bindings, 1.5), RangeError);
  assert.throws(() => createSemanticInputV2Recorder([{ action: 'jump', input: 'tilt' }]), TypeError);
  assert.throws(() => createSemanticInputV2Recorder([{ action: 'jump', input: 'tap' },
    { action: 'jump', input: 'drag' }]), TypeError);
  assert.throws(() => createSemanticInputV2Recorder([{ action: 'é'.repeat(65), input: 'tap' }]), TypeError);
  // Go's strict wire parser rejects unpaired surrogate escapes; no browser
  // replacement-rune normalization may turn them into a different action.
  assert.throws(() => createSemanticInputV2Recorder([{ action: '\uD800', input: 'tap' }]), TypeError);
  assert.throws(() => createSemanticInputV2Recorder([{ action: '\uDC00', input: 'keyboard' }]), TypeError);
  const malformed = createSemanticInputV2Recorder(bindings);
  assert.equal(malformed.record({
    tick: 1, action: '\uD800', device: 'pointer', phase: 'down', position: fixed(0, 0),
  }), false);
  assert.equal(malformed.complete, false);
  assert.equal(malformed.failureReason, 'invalid_event');
  assert.throws(() => createSemanticInputV2Recorder([{ action: 'jump', input: 'tap' },
    ...Array.from({ length: 256 }, (_, i) => ({ action: String(i), input: 'keyboard' }))]), RangeError);
});
