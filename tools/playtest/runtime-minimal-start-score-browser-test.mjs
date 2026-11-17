#!/usr/bin/env node
// Epic #202 / #478: execute genuine preinstalled Pixi with an intentionally
// narrow synthetic Game IR document. Playwright alters ONLY the declarative
// test package on loopback, NOT Pixi, score/state, rendering, or the probe.
// This is browser bootstrap DIAGNOSTIC evidence, never publication, a Go
// replay, or an independently verified score.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
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

// A strictly declared, otherwise empty Game IR 1.0. The Go strict preflight
// accepts this minimal SOURCE shape; the actual runtime boot remains the
// subject of THIS test, not an assumption.
const diagnosticIR = {
  irVersion: '1.0',
  capabilities: [],
  game: {
    id: gameId,
    title: 'Synthetic diagnostic start-score runtime',
    entryScene: 'scene',
    orientation: 'any',
  },
  assets: [],
  scenes: [{ id: 'scene', entities: [] }],
  inputBindings: [],
  rules: [
    { id: 'start_plus_3', when: { event: 'game_start' },
      actions: [{ type: 'add_score', value: 3 }] },
    { id: 'start_minus_2', when: { event: 'game_start' },
      actions: [{ type: 'add_score', value: -2 }] },
  ],
};
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
        transport: 'Playwright-intercepted synthetic Game IR; NOT published',
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
  browser: browserName, diagnosticOnly: true,
  genuinePublishedGameIR: false,
  independentlyTrustedScore: false,
  passed: reports.length === 2,
  reports,
}, null, 2));
console.log(JSON.stringify({
  browser: browserName, scenarios: reports.length,
  genuinePublishedGameIR: false, independentlyTrustedScore: false,
  passed: reports.length === 2,
}));
