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


test('even two matching incomplete or duplicated report inventories fail against checkout manifests', () => {
  const a = fixtures('chromium'), b = fixtures('webkit');
  a.gameIDs.pop();
  b.gameIDs.pop();
  assert.throws(() => compare(a, b), /inventory coverage differs from checked-in manifests/);
  const dupeA = fixtures('chromium'), dupeB = fixtures('webkit');
  dupeA.gameIDs[1] = dupeA.gameIDs[0];
  dupeB.gameIDs[1] = dupeB.gameIDs[0];
  assert.throws(() => compare(dupeA, dupeB), /duplicate game IDs/);
  const foreignA = fixtures('chromium'), foreignB = fixtures('webkit');
  foreignA.gameIDs[0] = 'foreign-not-in-checkout';
  foreignB.gameIDs[0] = 'foreign-not-in-checkout';
  assert.throws(() => compare(foreignA, foreignB), /inventory coverage differs from checked-in manifests/);
});

test('top-level seed and exact tick must agree across browsers and every reported game', () => {
  const invalidSeed = fixtures('chromium');
  invalidSeed.seed = -1;
  assert.throws(() => compare(invalidSeed, fixtures('webkit')), /invalid report seed/);
  const invalidTick = fixtures('webkit');
  invalidTick.simStep = 108001;
  assert.throws(() => compare(fixtures('chromium'), invalidTick), /invalid report fixed tick/);

  const sameBadHeaderA = fixtures('chromium'), sameBadHeaderB = fixtures('webkit');
  sameBadHeaderA.seed = 42;
  sameBadHeaderB.seed = 42;
  assert.throws(() => compare(sameBadHeaderA, sameBadHeaderB),
    /entry seed mismatch against report cohort/);
  const sameBadTickA = fixtures('chromium'), sameBadTickB = fixtures('webkit');
  sameBadTickA.simStep = 241;
  sameBadTickB.simStep = 241;
  assert.throws(() => compare(sameBadTickA, sameBadTickB),
    /entry fixed tick is not bound to report cohort/);

  const changedOnlyWebkit = fixtures('webkit');
  changedOnlyWebkit.seed = 13;
  changedOnlyWebkit.results.forEach(entry => { entry.seed = 13; });
  assert.throws(() => compare(fixtures('chromium'), changedOnlyWebkit),
    /cross-engine report seed mismatch/);
  const changedTickOnlyWebkit = fixtures('webkit');
  changedTickOnlyWebkit.simStep = 241;
  changedTickOnlyWebkit.results.forEach(entry => {
    entry.simStep = 241;
    entry.state.simStep = 241;
  });
  assert.throws(() => compare(fixtures('chromium'), changedTickOnlyWebkit),
    /cross-engine report fixed tick mismatch/);
});

test('a single game or viewport cannot silently escape its report cohort', () => {
  const wrongSeed = fixtures('webkit');
  wrongSeed.results[0].seed = 999;
  assert.throws(() => compare(fixtures('chromium'), wrongSeed),
    /entry seed mismatch against report cohort/);
  const wrongTick = fixtures('webkit');
  wrongTick.results[5].simStep = 241;
  wrongTick.results[5].state.simStep = 241;
  assert.throws(() => compare(fixtures('chromium'), wrongTick),
    /entry fixed tick is not bound to report cohort/);
});
