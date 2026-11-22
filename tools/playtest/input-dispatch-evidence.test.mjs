import test from 'node:test';
import assert from 'node:assert/strict';
import { recordAfterDispatch, recordAfterGesture } from './input-dispatch-evidence.mjs';

test('only a completed Playwright API dispatch is recorded', async () => {
  const events = [];
  assert.equal(await recordAfterDispatch(async () => undefined, () => events.push('tap')), true);
  assert.equal(await recordAfterDispatch(async () => false, () => events.push('missing-canvas')), false);
  assert.equal(await recordAfterDispatch(async () => { throw new Error('page closed'); }, () => events.push('failed')), false);
  assert.deepEqual(events, ['tap']);
});

test('a failed mouse/key gesture never fabricates an observed action but still releases', async () => {
  const events = [], steps = [];
  const result = await recordAfterGesture([
    async () => { steps.push('down'); },
    async () => { steps.push('move'); throw new Error('disconnected'); },
    async () => { steps.push('up'); }
  ], () => events.push('drag'));
  assert.equal(result, false);
  assert.deepEqual(steps, ['down', 'move', 'up']);
  assert.deepEqual(events, []);
});

test('completed sequence and synthetic dispatch sentinel record exactly once', async () => {
  const events = [];
  assert.equal(await recordAfterGesture([
    async () => true, async () => undefined, async () => true
  ], () => events.push('drag')), true);
  assert.equal(await recordAfterGesture([
    async () => false, async () => true
  ], () => events.push('synthetic')), false);
  assert.deepEqual(events, ['drag']);
});
