#!/usr/bin/env node
// Epic #202/#315: DOM event floods must never execute authored rules outside
// the canonical 60Hz semantic tick or fabricate a verified/winning result.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/runtime-input-queue');
mkdirSync(shots, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const results = [];
try {
  for (const device of [
    { name: 'iphone-landscape', width: 844, height: 390 },
    { name: 'ipad-landscape', width: 1180, height: 820 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: device.width, height: device.height }, hasTouch: true,
    });
    const uncaught = [];
    page.on('pageerror', e => uncaught.push(String(e)));
    try {
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1',
        { timeout: 30000 });
      await page.waitForFunction(() => Boolean(window.__dqTest?.snapshot), null, { timeout: 30000 });
      const start = page.locator('#shell-start-btn');
      if (await start.isVisible().catch(() => false)) await start.click();
      await page.waitForFunction(() => window.__dqTest?.snapshot()?.started === true,
        null, { timeout: 20000 });
      // Evaluate in one synchronous browser task: no requestAnimationFrame or
      // semantic tick may intervene to silently discard part of the flood.
      const events = await page.evaluate(() => {
        const canvas = document.querySelector('canvas');
        if (!canvas) throw new Error('runtime canvas missing');
        const b = canvas.getBoundingClientRect();
        for (let i = 0; i < 129; i++) {
          canvas.dispatchEvent(new PointerEvent('pointerdown', {
            bubbles: true, clientX: b.left + b.width / 2,
            clientY: b.top + b.height / 2, pointerType: 'touch', pointerId: i + 1,
          }));
        }
        return 129;
      });
      assert.equal(events, 129);
      await page.waitForFunction(() => window.__dqTest?.snapshot()?.failed === true,
        null, { timeout: 10000 });
      const state = await page.evaluate(() => window.__dqTest.snapshot());
      assert.equal(state.failed, true, 'flood must trigger fail-closed runtime state');
      assert.equal(state.ended, false, 'flood must never end as a completed game');
      assert.equal(state.won, false, 'flood must never fabricate a victory');
      assert.equal(await page.locator('#shell-result.on').count(), 0);
      assert.match(await page.locator('#status').textContent(), /Game cannot continue safely/);
      assert.deepEqual(uncaught, [], 'input flood must not raise an uncaught DOM error');
      const screenshot = shots + '/' + browserName + '-' + device.name + '.png';
      await page.screenshot({ path: screenshot });
      results.push({ browser: browserName, device: device.name, dispatched: events,
        failed: true, ended: false, won: false, screenshot });
    } finally { await page.close(); }
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, passed: results.length === 2, results,
}, null, 2));
console.log(JSON.stringify({ browser: browserName, passed: true, scenarios: results.length }));
