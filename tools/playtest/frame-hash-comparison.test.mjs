import test from 'node:test';
import assert from 'node:assert/strict';
import { compareFrameHashes } from './frame-hash-comparison.mjs';

const a = 'a'.repeat(64);
const b = 'b'.repeat(64);
const c = 'c'.repeat(64);

test('matching repeated renders pass with the complete frame count', () => {
  assert.deepEqual(compareFrameHashes([a, b, c], [a, b, c]), {
    match: true, frame_count: 3, first_mismatch_index: null,
  });
});

test('the first changed frame is reported deterministically', () => {
  assert.deepEqual(compareFrameHashes([a, b, c], [a, c, c]), {
    match: false, frame_count: 3, first_mismatch_index: 1,
  });
});

test('a frame-count difference is a mismatch', () => {
  assert.deepEqual(compareFrameHashes([a, b], [a, b, c]), {
    match: false, frame_count: 2, first_mismatch_index: 2,
  });
});

test('empty or malformed hash reports are rejected', () => {
  assert.throws(() => compareFrameHashes([], [a]), /non-empty/);
  assert.throws(() => compareFrameHashes([a], ['not-a-hash']), /SHA-256/);
});
