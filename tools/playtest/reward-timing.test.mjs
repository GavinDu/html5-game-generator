import test from 'node:test';
import assert from 'node:assert/strict';
import { mergeFirstRewardMeasurement, selectFirstRewardMeasurement } from './reward-timing.mjs';

test('runtime visible-play timing wins over delayed HUD polling', () => {
  assert.deepEqual(
    selectFirstRewardMeasurement(1004, true, 6404),
    { ms: 1004, source: 'runtime probe' },
  );
});

test('HUD timing is used only when the runtime has no reward measurement', () => {
  assert.deepEqual(
    selectFirstRewardMeasurement(null, true, 2500),
    { ms: 2500, source: 'HUD score observation' },
  );
});

test('missing runtime and HUD evidence remains unmeasured', () => {
  assert.equal(selectFirstRewardMeasurement(null, false, 1000), null);
});

test('selected first-reward timing is copied into the probe snapshot used by scoring', () => {
  const probe = { firstRewardMs: null, score: 1 };
  const measured = selectFirstRewardMeasurement(1004, true, 6404);
  const updated = mergeFirstRewardMeasurement(probe, measured);

  assert.equal(updated.firstRewardMs, 1004);
  assert.equal(updated.score, 1);
  assert.equal(probe.firstRewardMs, null);
});

test('missing or invalid timing leaves the probe snapshot unchanged', () => {
  const probe = { firstRewardMs: null };
  assert.equal(mergeFirstRewardMeasurement(probe, null), probe);
  assert.equal(mergeFirstRewardMeasurement(probe, { ms: -1 }), probe);
});
