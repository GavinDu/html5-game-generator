#!/usr/bin/env node
// Actual short touch contacts must move the catcher; missed hearts must return.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
const arg = (key, fallback) => { const i = process.argv.indexOf('--' + key); return i >= 0 ? process.argv[i + 1] : fallback; };
const base = arg('base', 'http://localhost:18099');
const shots = arg('shots', '/tmp/bobo-controls');
const engine = arg('browser', 'chromium') === 'webkit' ? webkit : chromium;
mkdirSync(shots, { recursive: true });
const browser = await engine.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true });
  await page.goto(base + '/player?game=sample-rainbow-bear&presentation=play&probe=1');
  await page.waitForFunction(() => window.__dqTest?.snapshot?.().t > 0.2);
  const before = await page.evaluate(() => window.__dqTest.snapshot());
  const rect = await page.locator('#stage canvas').boundingBox();
  // A single short tap, with no held-contact fallback or runtime state writes.
  await page.touchscreen.tap(rect.x + rect.width * 0.85, rect.y + rect.height * 0.6);
  await page.waitForFunction(x => window.__dqTest.snapshot().player.x > x + 40, before.player.x, { timeout: 3000 });
  const moved = await page.evaluate(() => window.__dqTest.snapshot());
  assert.ok(Math.abs(moved.player.y - before.player.y) < 2, 'catcher must stay in the horizontal lane');
  await page.screenshot({ path: shots + '/after-short-tap.png' });
  // Leave the blue heart alone on the other side, observe it fall, then return
  // to the top. Horizontal sway must not keep a missed heart stuck forever.
  await page.waitForFunction(() => window.__dqTest.snapshot().entities.some(e => e.id === 'heart_blue' && e.y > 770), null, { timeout: 10000 });
  const low = await page.evaluate(() => window.__dqTest.snapshot().entities.find(e => e.id === 'heart_blue'));
  await page.screenshot({ path: shots + '/missed-heart-before-recycle.png' });
  await page.waitForFunction(() => window.__dqTest.snapshot().entities.some(e => e.id === 'heart_blue' && e.y < 100), null, { timeout: 12000 });
  const recycled = await page.evaluate(() => window.__dqTest.snapshot().entities.find(e => e.id === 'heart_blue'));
  await page.screenshot({ path: shots + '/missed-heart-after-recycle.png' });
  const report = { passed: true, gesture: 'real touchscreen tap', before: before.player, after: moved.player, low, recycled };
  writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report));
} finally { await browser.close(); }
