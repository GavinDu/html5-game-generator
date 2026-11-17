#!/usr/bin/env node
// Cross-engine semantic replay equality across *all* shipped Game IR.
// Never compare GPU pixel hashes: those may differ without game-state drift.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Inventory coverage is derived from the checked-in manifest directory, not
// from a browser-supplied list. This is source-controlled TEST provenance,
// not an authenticated remote published GamePackage or trace.
function shippedInventoryGameIDs() {
  const directory = new URL('../../inventory/', import.meta.url);
  const ids = readdirSync(directory, { withFileTypes: true })
    .filter(entry => entry.isDirectory())
    .map(entry => {
      const manifest = JSON.parse(readFileSync(
        new URL(entry.name + '/manifest.json', directory), 'utf8'
      ));
      assert.match(manifest.game_id, /^[a-z0-9][a-z0-9-]{0,127}$/,
        'every shipped inventory directory must declare a canonical game ID');
      return manifest.game_id;
    }).sort();
  assert.ok(ids.length >= 4, 'expected all checked-in inventory games');
  assert.equal(new Set(ids).size, ids.length, 'duplicate shipped inventory game ID');
  return ids;
}

function validateReportCohort(report, name, expectedIDs) {
  assert.ok(Number.isSafeInteger(report.seed) &&
    report.seed >= 0 && report.seed <= 0xffffffff,
  name + ' has an invalid report seed');
  assert.ok(Number.isSafeInteger(report.simStep) &&
    report.simStep >= 1 && report.simStep <= 108000,
  name + ' has an invalid report fixed tick');
  assert.ok(Array.isArray(report.gameIDs), name + ' is missing game IDs');
  assert.equal(new Set(report.gameIDs).size, report.gameIDs.length,
    name + ' contains duplicate game IDs');
  assert.deepEqual(report.gameIDs, expectedIDs,
    name + ' inventory coverage differs from checked-in manifests');
}

export function compareInventorySemanticReplay(chromium, webkit) {
  assert.equal(chromium.browser, 'chromium');
  assert.equal(webkit.browser, 'webkit');
  assert.equal(chromium.passed, true);
  assert.equal(webkit.passed, true);
  const expectedIDs = shippedInventoryGameIDs();
  validateReportCohort(chromium, 'Chromium', expectedIDs);
  validateReportCohort(webkit, 'WebKit', expectedIDs);
  assert.equal(webkit.seed, chromium.seed, 'cross-engine report seed mismatch');
  assert.equal(webkit.simStep, chromium.simStep,
    'cross-engine report fixed tick mismatch');
  const expected = new Set(expectedIDs.flatMap(game =>
    ['iphone-portrait', 'ipad-portrait'].map(v => game + ':' + v)));
  const states = (report, engine) => {
    const map = new Map();
    assert.ok(Array.isArray(report.results));
    for (const entry of report.results) {
      const key = entry.game + ':' + entry.viewport;
      assert.ok(expected.has(key), 'unexpected game/profile ' + key);
      assert.ok(!map.has(key), 'duplicate game/profile ' + key);
      assert.equal(entry.passed, true, engine + ' reported failed gameplay');
      assert.equal(entry.seed, report.seed,
        engine + ' entry seed mismatch against report cohort: ' + key);
      assert.equal(entry.simStep, report.simStep,
        engine + ' entry fixed tick is not bound to report cohort: ' + key);
      assert.equal(entry.state?.simStep, entry.simStep, 'not exact fixed tick');
      map.set(key, entry);
    }
    assert.deepEqual([...map.keys()].sort(), [...expected].sort(),
      'missing one or more shipped game profiles from ' + engine);
    return map;
  };
  const a = states(chromium, 'Chromium'), b = states(webkit, 'WebKit');
  const matched = [];
  for (const [key, reference] of a) {
    const observed = b.get(key);
    assert.equal(observed.seed, reference.seed, 'seed mismatch: ' + key);
    assert.equal(observed.simStep, reference.simStep, 'tick mismatch: ' + key);
    assert.deepEqual(observed.state, reference.state,
      'cross-engine semantic-state divergence: ' + key);
    matched.push({ game: reference.game, viewport: reference.viewport,
      seed: reference.seed, simStep: reference.simStep, identical: true });
  }
  return { passed: true, verified: matched.length, matched };
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [, , chromPath, webkitPath] = process.argv;
  if (!chromPath || !webkitPath) throw new Error('pass chromium and webkit report paths');
  const chromium = JSON.parse(readFileSync(chromPath, 'utf8'));
  const webkit = JSON.parse(readFileSync(webkitPath, 'utf8'));
  console.log(JSON.stringify(compareInventorySemanticReplay(chromium, webkit), null, 2));
}
