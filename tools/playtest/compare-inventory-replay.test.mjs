import test from 'node:test';
import assert from 'node:assert/strict';
import { compareInventorySemanticReplay as compare } from './compare-inventory-replay.mjs';

const games = ['escape-from-injection', 'penguin-heartlight-rescue',
  'sample-rainbow-bear', 'sample-ribbon-sky'];
const fixtures = browser => ({
  browser, passed: true, seed: 271828, simStep: 240, gameIDs: games,
  results: games.flatMap(game =>
    ['iphone-portrait', 'ipad-portrait'].map(viewport => ({
      game, viewport, passed: true, seed: 271828, simStep: 240,
      state: { simStep: 240, score: 30, health: 1, ended: false,
        variables: { hearts_collected: 2 },
        entities: [{ id: 'sprite-1', def: 'heart', kind: 'pickup', x: 10, y: 20 }] },
    }))),
});

test('all inventory game/profile evidence matches across browsers', () => {
  assert.equal(compare(fixtures('chromium'), fixtures('webkit')).verified, 8);
});
test('a single score drift fails instead of being hidden by screenshot similarity', () => {
  const a = fixtures('chromium'), b = fixtures('webkit');
  b.results[3].state.score++;
  assert.throws(() => compare(a, b), /cross-engine semantic-state divergence/);
});
test('missing or duplicate game evidence cannot pass', () => {
  const a = fixtures('chromium'), b = fixtures('webkit');
  b.results.pop();
  assert.throws(() => compare(a, b), /missing one or more shipped game profiles/);
  b.results = [...a.results, a.results[0]];
  assert.throws(() => compare(a, b), /duplicate game\/profile/);
});
test('seed and simulation-step mismatches are prohibited', () => {
  const a = fixtures('chromium'), b = fixtures('webkit');
  b.results[0].seed = 13;
  assert.throws(() => compare(a, b), /seed mismatch/);
  b.results[0].seed = 271828;
  b.results[0].state.simStep = 241;
  assert.throws(() => compare(a, b), /not exact fixed tick/);
});
