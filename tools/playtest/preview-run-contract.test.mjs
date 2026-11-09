import test from 'node:test';
import assert from 'node:assert/strict';
import { validateRecordedRun } from './preview-run-contract.mjs';

const options = { game: 'penguin-heartlight-rescue', run: 'run-123', irHash: '0123456789ab' };
const valid = { id: options.run, gameId: options.game, irHash: options.irHash,
  seed: 271828, inputs: [{ tick: 12, type: 'pointer', detail: '100,200' }] };

test('accepts an exact recorded game/version replay, including an input-free idle run', () => {
  assert.equal(validateRecordedRun(valid, options), valid);
  assert.doesNotThrow(() => validateRecordedRun({ ...valid, inputs: undefined }, options));
});

test('rejects a wrong game, stale package IR, or a different run ID', () => {
  assert.throws(() => validateRecordedRun({ ...valid, gameId: 'another-game' }, options), /different game/);
  assert.throws(() => validateRecordedRun({ ...valid, irHash: 'old-version' }, options), /IR hash/);
  assert.throws(() => validateRecordedRun({ ...valid, id: 'another-run' }, options), /ID does not match/);
});

test('rejects missing recordings, malformed seeds and invalid trace envelopes', () => {
  assert.throws(() => validateRecordedRun(null, options), /not an object/);
  assert.throws(() => validateRecordedRun({ ...valid, seed: -1 }, options), /deterministic seed/);
  assert.throws(() => validateRecordedRun({ ...valid, seed: 2 ** 32 }, options), /deterministic seed/);
  assert.throws(() => validateRecordedRun({ ...valid, seed: 1.25 }, options), /deterministic seed/);
  assert.throws(() => validateRecordedRun({ ...valid, inputs: '[]' }, options), /input trace/);
});
