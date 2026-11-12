#!/usr/bin/env node
// Runtime Epic #202 / #315: compare *semantic* state at an exact fixed tick
// across Chromium and WebKit. Pixel hashes are deliberately NOT compared:
// GPU/antialiasing output is allowed to vary, but gameplay must not.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

export function compareCrossBrowserReplay(chromium, webkit) {
  assert.equal(chromium.browser, 'chromium', 'first report must be Chromium');
  assert.equal(webkit.browser, 'webkit', 'second report must be WebKit');
  assert.equal(chromium.passed, true, 'Chromium reported unplayable replay');
  assert.equal(webkit.passed, true, 'WebKit reported unplayable replay');
  const baseline = chromium.results;
  const candidate = webkit.results;
  assert.ok(Array.isArray(baseline) && baseline.length > 0, 'no Chromium replay states');
  assert.ok(Array.isArray(candidate) && candidate.length > 0, 'no WebKit replay states');

  function profiles(results, engine) {
    const byViewport = new Map();
    for (const r of results) {
      assert.equal(r.passed, true, engine + ' reported failed profile');
      assert.ok(typeof r.viewport === 'string' && r.viewport, 'missing viewport');
      assert.ok(!byViewport.has(r.viewport), 'duplicate viewport: ' + r.viewport);
      assert.ok(Number.isSafeInteger(r.seed) && Number.isSafeInteger(r.simStep),
        'missing seed or exact fixed simulation step');
      assert.ok(r.state && typeof r.state === 'object', 'missing actual semantic state');
      assert.equal(r.state.step, r.simStep, 'state not captured at reported simulation step');
      byViewport.set(r.viewport, r);
    }
    return byViewport;
  }
  const chromiumStates = profiles(baseline, 'Chromium');
  const webkitStates = profiles(candidate, 'WebKit');
  assert.deepEqual([...webkitStates.keys()].sort(), [...chromiumStates.keys()].sort(),
    'cross-browser profile list differs');

  const matched = [];
  for (const [viewport, reference] of chromiumStates) {
    const observed = webkitStates.get(viewport);
    assert.equal(observed.seed, reference.seed, 'seed differs: ' + viewport);
    assert.equal(observed.simStep, reference.simStep, 'fixed tick differs: ' + viewport);
    // The originating Escape test normalizes positions to 0.001 logical px.
    // Deliberately fail on any remaining semantic drift rather than silently
    // broadening a tolerance or treating screenshot resemblance as proof.
    assert.deepEqual(observed.state, reference.state,
      'seeded gameplay diverged between Chromium and WebKit: ' + viewport);
    matched.push({ viewport, seed: reference.seed, simStep: reference.simStep, identical: true });
  }
  return { passed: true, mode: 'exact-cross-browser-semantic-state', matched };
}

function main() {
  const [, , chromiumPath, webkitPath] = process.argv;
  if (!chromiumPath || !webkitPath) {
    throw new Error('usage: node compare-cross-browser-replay.mjs <chromium-report.json> <webkit-report.json>');
  }
  const chromium = JSON.parse(readFileSync(chromiumPath, 'utf8'));
  const webkit = JSON.parse(readFileSync(webkitPath, 'utf8'));
  process.stdout.write(JSON.stringify(compareCrossBrowserReplay(chromium, webkit), null, 2) + '\n');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main();
}
