import test from 'node:test';
import assert from 'node:assert/strict';
import { selectFirstRewardMeasurement } from './reward-timing.mjs';

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
