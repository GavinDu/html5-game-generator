#!/usr/bin/env node
// Epic #202/#316: real Pixi/Go Game Package delivery negative preflight.
// Tests are deliberately isolated to a browser probe route, never published
// content. No game-provided script is executed.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (key, fallback) => {
  const i = process.argv.indexOf('--' + key);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/capability-reject');
mkdirSync(shots, { recursive: true });
const engine = browserName === 'webkit' ? webkit : chromium;
const browser = await engine.launch({ headless: true });
const gameId = 'escape-from-injection';
const records = [];
const cases = [
  {
    name: 'unknown',
    edit: ir => { ir.capabilities.push('script.untrusted.v1'); },
    expected: 'unsupported or duplicate Game IR capability',
  },
  {
    name: 'duplicate',
    edit: ir => { ir.capabilities.push(ir.capabilities[0]); },
    expected: 'unsupported or duplicate Game IR capability',
  },
  {
    name: 'no-shell',
    edit: ir => { ir.capabilities = ir.capabilities.filter(x => x !== 'shell.v1'); },
    expected: 'Game IR 1.2 requires shell.v1 capability',
  },
];

try {
  for (const scenario of cases) {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true });
    try {
      await page.route('**/v1/games/*/package', async route => {
        const original = await route.fetch();
        assert.equal(original.status(), 200, 'baseline package must deliver');
        const pkg = await original.json();
        assert.equal(pkg.gameId, gameId);
        assert.equal(pkg.ir.irVersion, '1.2');
        assert.ok(Array.isArray(pkg.ir.capabilities));
        scenario.edit(pkg.ir);
        // This is a Playwright *negative* fixture only; the real backend
        // validator would reject the tampered IR before publishing.
        await route.fulfill({ response: original, json: pkg });
      });
      await page.goto(base + '/player?game=' + gameId + '&presentation=play&probe=1', { waitUntil: 'load', timeout: 30000 });
      await page.waitForFunction(error => {
        return document.querySelector('#status')?.textContent?.includes(error);
      }, scenario.expected, { timeout: 20000 });
      const message = await page.locator('#status').textContent();
      assert.match(message, /Failed to load:/);
      assert.ok(message.includes(scenario.expected));
      assert.equal(await page.locator('#stage canvas').count(), 0, 'unsupported capability must fail before Pixi allocation');
      assert.equal(await page.evaluate(() => !!window.__dqTest?.snapshot), false,
        'unsupported capability must not expose a running game');
      const screenshot = shots + '/' + browserName + '-' + scenario.name + '.png';
      await page.screenshot({ path: screenshot, fullPage: true });
      records.push({ name: scenario.name, passed: true, screenshot });
    } finally { await page.close(); }
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, passed: records.length === cases.length, records,
}, null, 2));
console.log(JSON.stringify({ browser: browserName, cases: records.map(r => r.name), passed: records.length === cases.length }));
