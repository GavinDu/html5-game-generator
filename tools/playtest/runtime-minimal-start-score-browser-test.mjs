#!/usr/bin/env node
// Epic #202 / #478: execute genuine preinstalled Pixi with an intentionally
// narrow synthetic Game IR document. Playwright alters ONLY the declarative
// test package on loopback, NOT Pixi, score/state, rendering, or the probe.
// This is browser bootstrap DIAGNOSTIC evidence, never publication, a Go
// replay, or an independently verified score.
import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/runtime-minimal-start-score');
const gameId = 'escape-from-injection';
mkdirSync(shots, { recursive: true });

// Load the EXACT SAME static Game IR source fixture as Go's strict
// internal/attestation/start_score_diagnostic_test.go. Route interception
// injects ONLY declarative Game IR, not Pixi code, state, score or a
// Playwright-forged probe. It is NOT an authenticated published snapshot.
const sharedFixtureBytes = readFileSync(
  new URL('../../internal/attestation/testdata/start_score_ir_v1.json', import.meta.url)
);
const fixtureSHA256 = createHash('sha256')
  .update(sharedFixtureBytes).digest('hex');
const diagnosticIR = JSON.parse(sharedFixtureBytes.toString('utf8'));

// Explicitly pin the minimal diagnostic capability and authored event order.
// If Go's fixture drifts to new gameplay semantics, the real Pixi browser
// cannot silently continue testing a different fake document.
assert.equal(diagnosticIR.irVersion, '1.0');
assert.equal(diagnosticIR.game.id, gameId,
  'the shared Go/Pixi Game IR game ID must match the loader envelope');
assert.deepEqual(diagnosticIR.capabilities, []);
assert.equal(diagnosticIR.scenes.length, 1);
assert.equal(diagnosticIR.scenes[0].id, 'scene');
assert.deepEqual(diagnosticIR.scenes[0].entities, []);
assert.deepEqual(diagnosticIR.assets, []);
assert.deepEqual(diagnosticIR.inputBindings, []);
assert.deepEqual(diagnosticIR.rules.map(rule => [
  rule.id, rule.when.event, rule.actions.map(action => [action.type, action.value]),
]), [
  ['start_plus', 'game_start', [['add_score', 3]]],
  ['start_minus', 'game_start', [['add_score', -2]]],
]);
assert.match(fixtureSHA256, /^[0-9a-f]{64}$/);

const browser = await (browserName === 'webkit' ? webkit : chromium)
  .launch({ headless: true });
const reports = [];

try {
  for (const device of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: device.width, height: device.height },
      hasTouch: true,
    });
    const uncaught = [];
    let intercepted = 0;
    page.on('pageerror', error => uncaught.push(String(error)));
    try {
      await page.route('**/v1/games/*/package', async route => {
        const upstream = await route.fetch();
        assert.equal(upstream.status(), 200,
          'real published game envelope is needed for the loader');
        const pkg = await upstream.json();
        assert.equal(pkg.gameId, gameId);
        // This is an isolated **test-only** synthetic IR, not a published
        // package or an authoritative signed package. Do not reuse the
        // upstream digest to identify different injected document bytes.
        pkg.ir = structuredClone(diagnosticIR);
        pkg.irVersion = '1.0';
        pkg.orientation = 'any';
        pkg.assets = [];
        pkg.irHash = '';
        pkg.irSha256 = '';
        intercepted++;
        await route.fulfill({ response: upstream, json: pkg });
      });

      const target = base +
        '/player?game=' + encodeURIComponent(gameId) +
        '&presentation=play&probe=1&seed=17';
      // The read-only probe is created only AFTER real preinstalled runtime
      // rule execution and the first render submission.
      const snapshots = [];
      for (let cycle = 0; cycle < 2; cycle++) {
        if (cycle === 0) {
          await page.goto(target, { waitUntil: 'load', timeout: 30000 });
        } else {
          await page.reload({ waitUntil: 'load', timeout: 30000 });
        }
        await page.waitForFunction(() => {
          const state = window.__dqTest?.snapshot?.();
          return state?.started === true && state?.frames >= 2 && state?.simStep >= 1;
        }, null, { timeout: 30000 });
        const first = await page.evaluate(() => window.__dqTest.snapshot());
        assert.equal(first.score, 1,
          'real Pixi game_start must execute +3 and -2 once');
        assert.equal(first.started, true);
        assert.equal(first.failed, false, 'synthetic v1 game must really boot');
        assert.equal(first.ended, false, 'no terminal action is authored');
        assert.equal(first.won, false, 'no synthetic win is authored');
        assert.equal(first.player, null, 'no player entity is authored');
        assert.deepEqual(first.entities, [], 'no entity should be fabricated');
        assert.equal(first.pendingSemanticInputs, 0);
        assert.ok(first.simStep >= 1 && first.frames >= 2,
          'real fixed ticker and display frames must have executed');
        assert.ok((await page.locator('#stage canvas').count()) >= 1,
          'actual Pixi canvas must exist');
        assert.equal(await page.locator('#shell-result.on').count(), 0,
          'a start-score diagnostic is never a finished run');
        const previousStep = first.simStep;
        await page.waitForFunction(step =>
          (window.__dqTest?.snapshot?.()?.simStep ?? 0) > step,
        previousStep, { timeout: 20000 });
        const later = await page.evaluate(() => window.__dqTest.snapshot());
        assert.equal(later.score, 1,
          'game_start must not be replayed on later fixed ticks');
        assert.equal(later.failed, false);
        assert.equal(later.ended, false);
        const screenshot = shots + '/' + browserName + '-' + device.name +
          '-cycle-' + cycle + '.png';
        await page.screenshot({ path: screenshot, fullPage: true });
        snapshots.push({
          cycle, score: later.score, firstStep: previousStep,
          lastStep: later.simStep, frames: later.frames,
          failed: later.failed, ended: later.ended, won: later.won,
          screenshot,
        });
      }
      assert.equal(intercepted, 2,
        'each genuine new page document must fetch a fresh synthetic fixture');
      assert.deepEqual(uncaught, [], 'real Pixi must have no uncaught page errors');
      reports.push({
        browser: browserName, device: device.name,
        transport: 'Playwright-intercepted shared synthetic Go/Pixi Game IR; NOT published',
        fixtureSHA256,
        scenario: 'one real game_start, +3 then -2, no inputs',
        snapshots, passed: true,
      });
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}
writeFileSync(shots + '/report.json', JSON.stringify({
  schema: 'diagnostic-pixi-bootstrap-v1',
  fixtureSHA256, sharedSourceGoAndPixi: true,
  browser: browserName, diagnosticOnly: true,
  genuinePublishedGameIR: false,
  independentlyTrustedScore: false,
  passed: reports.length === 2,
  reports,
}, null, 2));
console.log(JSON.stringify({
  browser: browserName, scenarios: reports.length, fixtureSHA256,
  genuinePublishedGameIR: false, independentlyTrustedScore: false,
  passed: reports.length === 2,
}));
