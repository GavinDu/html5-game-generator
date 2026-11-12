#!/usr/bin/env node
// Epic #202 negative conformance: an untrusted package with 8,193
// game-start actions must not monopolize the ticker or fabricate a victory.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const out = arg('shots', '/tmp/work-budget');
mkdirSync(out, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const report = [];
try {
  for (const viewport of [
    { name: 'iphone', width: 390, height: 844 },
    { name: 'ipad', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: viewport.width, height: viewport.height }, hasTouch: true,
    });
    const errors = [];
    page.on('pageerror', e => errors.push(String(e)));
    try {
      await page.route('**/v1/games/*/package', async route => {
        const upstream = await route.fetch();
        assert.equal(upstream.status(), 200);
        const pkg = await upstream.json();
        assert.equal(pkg.gameId, 'escape-from-injection');
        pkg.ir.rules = [{
          id: 'synthetic-unbounded-game-start',
          when: { event: 'game_start' },
          actions: Array.from({ length: 8193 },
            () => ({ type: 'add_score', value: 1 })),
        }];
        await route.fulfill({ response: upstream, json: pkg });
      });
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1',
        { timeout: 30000 });
      const start = page.locator('#shell-start-btn');
      if (await start.isVisible().catch(() => false)) await start.click();
      await page.waitForFunction(() =>
        document.querySelector('#status')?.textContent
          ?.includes('Game cannot continue safely'), null, { timeout: 20000 });
      const snapshot = await page.evaluate(() => window.__dqTest?.snapshot?.() || null);
      assert.equal(snapshot?.ended, false, 'budget violation cannot report a terminal win');
      assert.equal(snapshot?.won, false, 'budget violation cannot fabricate victory');
      assert.equal(await page.locator('#shell-result.on').count(), 0,
        'result card must not claim a completed run');
      assert.deepEqual(errors, [], 'expected budget failure is handled without an uncaught error');
      const shot = out + '/' + browserName + '-' + viewport.name + '.png';
      await page.screenshot({ path: shot });
      report.push({ browser: browserName, viewport: viewport.name,
        rejectedActions: 8193, ended: false, won: false, passed: true, screenshot: shot });
    } finally { await page.close(); }
  }
} finally { await browser.close(); }
writeFileSync(out + '/report.json', JSON.stringify({
  browser: browserName, passed: report.length === 2, reports: report,
}, null, 2));
console.log(JSON.stringify({ browser: browserName, passed: true, scenarios: report.length }));
