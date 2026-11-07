import test from 'node:test';
import assert from 'node:assert/strict';
import { outcomeContract } from '../outcome-contract.mjs';

const caught = {
  controls: { scheme: 'lane_tap' },
  chaserGap: { start: 220, max: 250, caughtAt: 55, recoveryPerSecond: 6 },
  result: { lose: { title: 'Caught!' } },
};
const loss = [{ id: 'gap-caught', actions: [{ type: 'end_game', value: false }] }];

test('authored caught-only chase is not mislabeled an unwinnable conventional game', () => {
  assert.deepEqual(outcomeContract(loss, caught),
    { hasWin: false, hasLoss: true, lossOnlyChase: true });
});

test('normal games still require a win rule', () => {
  assert.deepEqual(outcomeContract([{ actions: [{ type: 'end_game', value: true }] }], {}),
    { hasWin: true, hasLoss: false, lossOnlyChase: false });
  assert.deepEqual(outcomeContract(loss, {}),
    { hasWin: false, hasLoss: true, lossOnlyChase: false });
});

test('a malformed caught story cannot bypass the real winner check', () => {
  assert.equal(outcomeContract([], caught).lossOnlyChase, false);
  assert.equal(outcomeContract(loss, { ...caught, result: {} }).lossOnlyChase, false);
  assert.equal(outcomeContract(loss, { ...caught, controls: { scheme: 'tap' } }).lossOnlyChase, false);
  assert.equal(outcomeContract(loss, { ...caught, chaserGap: { caughtAt: NaN } }).lossOnlyChase, false);
});
