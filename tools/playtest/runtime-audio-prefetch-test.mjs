#!/usr/bin/env node
// Epic #202/#205: required Penguin chime has to be decoded before offline
// stress, not fetched at the instant the first heart is collected.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const index = process.argv.indexOf('--' + name);
  return index >= 0 ? process.argv[index + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/runtime-audio-prefetch');
const engine = browserName === 'webkit' ? webkit : chromium;
mkdirSync(shots, { recursive: true });
const browser = await engine.launch({ headless: true });
const reports = [];
try {
  for (const viewport of [
    { name: 'iphone', width: 390, height: 844 },
    { name: 'ipad', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: viewport.width, height: viewport.height },
      hasTouch: true,
    });
    const errors = [], failedSounds = [];
    page.on('pageerror', err => errors.push(String(err)));
    page.on('requestfailed', request => {
      if (/\/assets\/chime/.test(request.url())) {
        failedSounds.push({
          url: request.url(), error: request.failure()?.errorText || 'unknown',
        });
      }
    });
    try {
      await page.goto(base + '/player?game=penguin-heartlight-rescue&presentation=play&probe=1', { timeout: 30000 });
      await page.waitForFunction(() => window.__dqTest?.snapshot?.(), null, { timeout: 25000 });
      const start = page.locator('#shell-start-btn');
      if (await start.isVisible()) await start.click();
      await page.waitForFunction(() =>
        window.__dqTest?.snapshot?.()?.decodedSoundIds?.includes('chime'),
        null, { timeout: 20000 });
      const beforeOffline = await page.evaluate(() => window.__dqTest.snapshot());
      assert.ok(beforeOffline.decodedSoundIds.includes('chime'),
        'frequent short SFX must decode before the offline stress window');
      await page.context().setOffline(true);
      await page.waitForTimeout(750);
      await page.context().setOffline(false);
      await page.waitForTimeout(400);
      const afterOffline = await page.evaluate(() => window.__dqTest.snapshot());
      assert.ok(afterOffline.decodedSoundIds.includes('chime'),
        'offline interruption must not discard already decoded chime');
      assert.equal(afterOffline.audioDecodeJobs <= 4, true,
        'at most four concurrent audio downloads/decodes');
      assert.deepEqual(failedSounds, [], 'required SFX predecode should not abort during offline window');
      assert.deepEqual(errors, [], 'no JavaScript errors during audio predecode');
      const screenshot = shots + '/' + browserName + '-' + viewport.name + '.png';
      await page.screenshot({ path: screenshot });
      reports.push({ browser: browserName, viewport: viewport.name, decoded: 'chime',
        jobs: afterOffline.audioDecodeJobs, screenshot, passed: true });
      console.log(JSON.stringify(reports.at(-1)));
    } finally { await page.close(); }
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({ browser: browserName,
  passed: reports.length === 2, reports }, null, 2));
