#!/usr/bin/env node
// Epic #202/#318: the same authored tap scheme can intentionally move in
// X or XY. The new capability must override any game-objective legacy rule.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/runtime-movement-axes');
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
mkdirSync(shots, { recursive: true });
const results = [];
try {
  for (const scenario of [
    // Penguin's old objective uses horizontal-only compatibility. Prove a
    // new declarative XY control can override it WITHOUT changing its name.
    { game: 'penguin-heartlight-rescue', axes: 'xy', moveY: true },
    // Ribbon's old objective uses XY compatibility. Explicit X must win.
    { game: 'sample-ribbon-sky', axes: 'x', moveY: false },
  ]) {
    for (const viewport of [
      { name: 'iphone-portrait', width: 390, height: 844 },
      { name: 'ipad-portrait', width: 820, height: 1180 },
    ]) {
      const page = await browser.newPage({
        viewport: { width: viewport.width, height: viewport.height },
        hasTouch: true, deviceScaleFactor: 1,
      });
      const errors = [];
      page.on('pageerror', e => errors.push(String(e)));
      try {
        await page.route('**/v1/games/*/package', async route => {
          const upstream = await route.fetch();
          assert.equal(upstream.status(), 200);
          const pkg = await upstream.json();
          assert.equal(pkg.gameId, scenario.game);
          assert.equal(pkg.ir.experience.controls.scheme, 'tap');
          assert.ok(!pkg.ir.capabilities.includes('input.movement_axes.v1'));
          pkg.ir.capabilities.push('input.movement_axes.v1');
          pkg.ir.experience.controls.movementAxes = scenario.axes;
          await route.fulfill({ response: upstream, json: pkg });
        });
        await page.goto(base + '/player?game=' + scenario.game + '&presentation=play&probe=1&seed=271828',
          { timeout: 30000 });
        await page.waitForFunction(() => !!window.__dqTest?.snapshot, null, { timeout: 30000 });
        const start = page.locator('#shell-start-btn');
        if (await start.isVisible().catch(() => false)) await start.click();
        await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.started === true,
          null, { timeout: 20000 });
        const before = await page.evaluate(() => window.__dqTest.snapshot());
        assert.equal(before.failed, false);
        assert.ok(before.player);
        const target = {
          x: Math.min(before.world.w - 70, before.player.x + 85),
          y: Math.max(before.player.h, before.player.y - 130),
        };
        await page.evaluate(({ target, world }) => {
          const canvas = document.querySelector('#stage canvas');
          if (!canvas) throw new Error('missing real Pixi canvas');
          const r = canvas.getBoundingClientRect();
          const ev = (name) => new PointerEvent(name, {
            bubbles: true, pointerType: 'touch', pointerId: 77,
            clientX: r.left + r.width * target.x / world.w,
            clientY: r.top + r.height * target.y / world.h,
          });
          canvas.dispatchEvent(ev('pointerdown'));
          canvas.dispatchEvent(ev('pointerup'));
        }, { target, world: before.world });
        await page.waitForFunction(({ x, y }) => {
          const s = window.__dqTest?.snapshot?.();
          return s?.player && Math.abs(s.player.x - x) >= 20 && !s.failed;
        }, { x: before.player.x, y: before.player.y }, { timeout: 15000 });
        const after = await page.evaluate(() => window.__dqTest.snapshot());
        const dx = after.player.x - before.player.x;
        const dy = after.player.y - before.player.y;
        assert.ok(Math.abs(dx) > 20, 'tap must move the character horizontally');
        if (scenario.moveY) {
          assert.ok(dy < -40, 'explicit xy must enable vertical movement even on Penguin');
        } else {
          assert.ok(Math.abs(dy) < 4, 'explicit x must forbid vertical movement even on Ribbon');
        }
        assert.equal(after.failed, false);
        assert.deepEqual(errors, [], 'runtime must not throw for valid declared axes');
        const screenshot = shots + '/' + browserName + '-' + scenario.game + '-' + viewport.name + '.png';
        await page.screenshot({ path: screenshot });
        results.push({ browser: browserName, game: scenario.game,
          viewport: viewport.name, movementAxes: scenario.axes, dx, dy,
          passed: true, screenshot });
      } finally { await page.close(); }
    }
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, passed: results.length === 4, results,
}, null, 2));
console.log(JSON.stringify({ browser: browserName, passed: true, scenarios: results.length }));
