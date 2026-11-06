import test from 'node:test';
import assert from 'node:assert/strict';
import { gameOrientation, preferredViewport, supportsViewport } from './game-orientation.mjs';
test('orientation flags select gameplay profiles without requiring an opposite layout', () => {
  assert.equal(supportsViewport('portrait', 390, 844), true);
  assert.equal(supportsViewport('portrait', 844, 390), false);
  assert.equal(supportsViewport('landscape', 844, 390), true);
  assert.equal(supportsViewport('landscape', 390, 844), false);
  assert.equal(supportsViewport('any', 390, 844), true);
  assert.equal(supportsViewport('any', 844, 390), true);
});
test('worker boots landscape games sideways and preserves authored any aspect', () => {
  assert.deepEqual(preferredViewport({ game: { orientation: 'landscape' } }), { width: 960, height: 430 });
  assert.deepEqual(preferredViewport({ game: { orientation: 'portrait' } }), { width: 430, height: 960 });
  assert.deepEqual(preferredViewport({ game: { orientation: 'any' }, scenes: [{ worldSize: [1200, 600] }] }), { width: 960, height: 430 });
  assert.throws(() => gameOrientation({ game: { orientation: 'sideways' } }), /Unsupported/);
});
