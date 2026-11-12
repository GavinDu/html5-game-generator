import test from 'node:test';
import assert from 'node:assert/strict';
import { createSeededRandom } from '../../internal/api/web/runtime-seeded-rng.mjs';

// Original monolithic algorithm retained as a frozen reference, to
// prevent a performance refactor from silently changing old replays.
function reference(seed) {
  let state = (seed >>> 0) || 1;
  return () => {
    state = (state + 0x6D2B79F5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

test('one thousand random draws match legacy behavior exactly, including zero and uint32 wrapping', () => {
  for (const seed of [0, 1, 42, 271828, 0x7fffffff, 0x80000000, 0xffffffff]) {
    const stream = createSeededRandom(seed);
    const legacy = reference(seed);
    for (let i = 0; i < 1000; i++) {
      assert.equal(stream.next(), legacy(), `seed ${seed}, draw ${i}`);
    }
  }
});

test('reseed reproduces the same initial sequence and never leaks other sessions', () => {
  const a = createSeededRandom(42);
  const b = createSeededRandom(42);
  for (let i = 0; i < 33; i++) assert.equal(a.next(), b.next());
  b.reseed(1);
  assert.notEqual(a.getState(), b.getState());
  a.reseed(42);
  const first = reference(42)();
  assert.equal(a.next(), first);
  assert.equal(createSeededRandom(0).next(), createSeededRandom(1).next());
});

test('state is a stable unsigned integer and never exposes mutable module globals', () => {
  const a = createSeededRandom(0xffffffff);
  const b = createSeededRandom(0xffffffff);
  const previous = a.getState();
  assert.equal(previous, 0xffffffff);
  a.next();
  assert.notEqual(a.getState(), previous);
  assert.equal(b.getState(), previous, 'independent streams must not share state');
  assert.ok(Number.isInteger(a.getState()) && a.getState() >= 0 && a.getState() <= 0xffffffff);
});
