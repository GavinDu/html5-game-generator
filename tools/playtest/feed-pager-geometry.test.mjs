import test from 'node:test';
import assert from 'node:assert/strict';
import { evaluateSettledPager } from './feed-pager-geometry.mjs';

const pager = { top: 60, bottom: 360 };
const cards = [
  { gameID: 'bobo', top: 60, bottom: 360 },
  { gameID: 'penguin', top: 360, bottom: 660 },
];

test('one exactly aligned card is settled; adjacent DOM pages outside viewport are fine', () => {
  assert.deepEqual(evaluateSettledPager(pager, cards), {
    aligned: true, visibleCount: 1, visibleGameIDs: ['bobo'],
    topOffsetPx: 0, bottomOffsetPx: 0, reason: 'settled',
  });
});

test('a neighboring game bleeding under the pager is rejected even if card height equals viewport', () => {
  const result = evaluateSettledPager(pager, [
    { gameID: 'bobo', top: 48, bottom: 348 },
    { gameID: 'penguin', top: 348, bottom: 648 },
  ]);
  assert.equal(result.aligned, false);
  assert.equal(result.visibleCount, 2);
  assert.deepEqual(result.visibleGameIDs, ['bobo', 'penguin']);
  assert.equal(result.reason, 'neighboring-card-visible');
});

test('the viewport, not the whole window, determines card visibility', () => {
  const result = evaluateSettledPager(pager, [
    { gameID: 'previous', top: -240, bottom: 60 },
    { gameID: 'bobo', top: 60, bottom: 360 },
    { gameID: 'next', top: 360, bottom: 660 },
  ]);
  assert.equal(result.aligned, true);
  assert.deepEqual(result.visibleGameIDs, ['bobo']);
});

test('tolerate subpixel scroll rounding but reject off-center settled page', () => {
  assert.equal(evaluateSettledPager(pager, [
    { gameID: 'bobo', top: 59.3, bottom: 359.3 },
    { gameID: 'next', top: 359.3, bottom: 659.3 },
  ]).aligned, true);
  assert.equal(evaluateSettledPager(pager, [
    { gameID: 'bobo', top: 56, bottom: 356 },
  ]).aligned, false);
});

test('empty, invalid, and multiple short cards cannot pass', () => {
  assert.equal(evaluateSettledPager(pager, []).aligned, false);
  assert.equal(evaluateSettledPager({ top: NaN, bottom: 360 }, cards).aligned, false);
  assert.equal(evaluateSettledPager(pager, [
    { gameID: 'a', top: 60, bottom: 210 },
    { gameID: 'b', top: 210, bottom: 360 },
  ]).visibleCount, 2);
});
