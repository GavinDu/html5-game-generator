#!/usr/bin/env node
// Cross-engine semantic replay equality across *all* shipped Game IR.
// Never compare GPU pixel hashes: those may differ without game-state drift.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export function compareInventorySemanticReplay(chromium, webkit) {
  assert.equal(chromium.browser, 'chromium');
  assert.equal(webkit.browser, 'webkit');
  assert.equal(chromium.passed, true);
  assert.equal(webkit.passed, true);
  assert.ok(Array.isArray(chromium.gameIDs) && chromium.gameIDs.length >= 4,
    'at least four published fixtures are required');
  assert.deepEqual(webkit.gameIDs, chromium.gameIDs, 'inventory coverage differed by engine');
  const expected = new Set(chromium.gameIDs.flatMap(game =>
    ['iphone-portrait', 'ipad-portrait'].map(v => game + ':' + v)));
  const states = (report, engine) => {
    const map = new Map();
    assert.ok(Array.isArray(report.results));
    for (const entry of report.results) {
      const key = entry.game + ':' + entry.viewport;
      assert.ok(expected.has(key), 'unexpected game/profile ' + key);
      assert.ok(!map.has(key), 'duplicate game/profile ' + key);
      assert.equal(entry.passed, true, engine + ' reported failed gameplay');
      assert.ok(Number.isSafeInteger(entry.seed) && Number.isSafeInteger(entry.simStep));
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
