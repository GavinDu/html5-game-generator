#!/usr/bin/env node
// Test-only mutation of real Game IR; no new game code or parser bypass.
// Required scene entity counts above the mobile limit must fail before Pixi.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const browserName = arg('browser', 'chromium');
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const out = arg('shots', '/tmp/entity-budget-rejection');
mkdirSync(out, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const reports = [];
try {
  for (const viewport of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: viewport.width, height: viewport.height },
      hasTouch: true,
    });
    try {
      await page.route('**/v1/games/*/package', async route => {
        const upstream = await route.fetch();
        assert.equal(upstream.status(), 200);
        const pkg = await upstream.json();
        assert.equal(pkg.gameId, 'escape-from-injection');
        const existing = pkg.ir.scenes[0].entities[0];
        pkg.ir.scenes[0].entities = Array.from({ length: 513 }, (_, i) =>
          ({ ...existing, id: 'budget-overflow-' + i }));
        await route.fulfill({ response: upstream, json: pkg });
      });
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1',
        { timeout: 30000 });
      await page.waitForFunction(() => document.querySelector('#status')?.textContent
        ?.includes('scene exceeds active entity budget before renderer initialization'),
        null, { timeout: 20000 });
      assert.equal(await page.locator('#stage canvas').count(), 0,
        'oversize scene must not allocate a Pixi canvas');
      assert.equal(await page.evaluate(() => !!window.__dqTest?.snapshot), false,
        'oversize scene must never reach game execution');
      const screenshot = out + '/' + browserName + '-' + viewport.name + '.png';
      await page.screenshot({ path: screenshot });
      reports.push({ browser: browserName, viewport: viewport.name, passed: true,
        oversizedEntities: 513, screenshot });
    } finally { await page.close(); }
  }
} finally { await browser.close(); }
writeFileSync(out + '/report.json', JSON.stringify({
  browser: browserName, passed: reports.length === 2, reports,
}, null, 2));
console.log(JSON.stringify({ browser: browserName, passed: true, count: reports.length }));
