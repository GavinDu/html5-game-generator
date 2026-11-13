#!/usr/bin/env node
// Epic #202 / #318. Real Pixi player in Chromium/WebKit, with the
// production game package; only read-only probe snapshots are inspected.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (key, fallback) => {
  const index = process.argv.indexOf('--' + key);
  return index >= 0 ? process.argv[index + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const output = arg('shots', '/tmp/input-cancellation');
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
mkdirSync(output, { recursive: true });

const cases = [];
try {
  for (const profile of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: profile.width, height: profile.height }, hasTouch: true,
    });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    try {
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1');
      await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.player, null, { timeout: 25000 });
      await page.evaluate(() => window.__dqBeginPlay?.());

      const state = () => page.evaluate(() => window.__dqTest.snapshot().heldInputs);
      const hold = () => page.evaluate(() => {
        const canvas = document.querySelector('#stage canvas');
        const bounds = canvas.getBoundingClientRect();
        canvas.dispatchEvent(new PointerEvent('pointerdown', {
          bubbles: true, pointerId: 17,
          clientX: bounds.left + bounds.width / 4,
          clientY: bounds.top + bounds.height / 2,
        }));
      });
      await hold();
      assert.equal((await state()).pointerDown, true, 'touch must register as held');

      await page.evaluate(() => window.dispatchEvent(new PointerEvent('pointercancel', { pointerId: 17 })));
      assert.equal((await state()).pointerDown, false, 'OS touch cancellation must release held input');

      await hold();
      assert.equal((await state()).pointerDown, true, 'second touch must start cleanly');
      await page.evaluate(() => window.dispatchEvent(new Event('blur')));
      assert.equal((await state()).pointerDown, false, 'WebView focus loss must release touch');

      await page.evaluate(() => window.dispatchEvent(new KeyboardEvent('keydown', {
        bubbles: true, key: 'ArrowLeft',
      })));
      assert.ok((await state()).keys.includes('move_left'), 'arrow press must activate bound action');
      await page.evaluate(() => window.dispatchEvent(new Event('blur')));
      assert.deepEqual((await state()).keys, [], 'WebView focus loss must release keyboard controls');

      await hold();
      await page.evaluate(() => document.querySelector('#stage canvas')
        .dispatchEvent(new Event('lostpointercapture')));
      assert.equal((await state()).pointerDown, false,
        'lost pointer capture must release a synthetic held touch');

      // A successful quick tap may get pointerup + lostpointercapture
      // before the next fixed tick. It is not a cancellation.
      const completedTapQueue = await page.evaluate(() => {
        // Synchronous dispatch avoids a real RAF tick consuming the tap
        // between the down/up/lostcapture events.
        const canvas = document.querySelector('#stage canvas');
        const bounds = canvas.getBoundingClientRect();
        canvas.dispatchEvent(new PointerEvent('pointerdown', {
          bubbles: true, pointerId: 18,
          clientX: bounds.left + bounds.width / 4,
          clientY: bounds.top + bounds.height / 2,
        }));
        window.dispatchEvent(new PointerEvent('pointerup', { pointerId: 18 }));
        canvas.dispatchEvent(new Event('lostpointercapture'));
        return window.__dqTest.snapshot().pendingSemanticInputs;
      });
      assert.ok(completedTapQueue >= 1,
        'normal implicit capture release must preserve a queued quick tap');
      await page.evaluate(() => window.dispatchEvent(new Event('blur')));
      assert.equal((await page.evaluate(() => window.__dqTest.snapshot().pendingSemanticInputs)), 0,
        'focus interruption must still cancel outstanding actions');

      // Rotation pauses the fixed-step simulation and revokes queued input.
      await hold();
      await page.setViewportSize({ width: profile.height, height: profile.width });
      await page.waitForFunction(() => document.body.dataset.orientationBlocked === 'true');
      assert.equal((await state()).pointerDown, false, 'rotation must release held input');
      const rotated = await page.evaluate(() => window.__dqTest.snapshot());
      assert.equal(rotated.pendingSemanticInputs, 0, 'rotation must discard queued commands');
      await page.waitForTimeout(160);
      assert.equal((await page.evaluate(() => window.__dqTest.snapshot().simStep)),
        rotated.simStep, 'simulation must not advance with incompatible orientation');
      await page.setViewportSize({ width: profile.width, height: profile.height });
      await page.waitForFunction(() => document.body.dataset.orientationBlocked === 'false');
      await page.waitForFunction(step => window.__dqTest.snapshot().simStep > step,
        rotated.simStep, { timeout: 5000 });

      assert.deepEqual(errors, [], 'no browser runtime exceptions during interruption flow');
      const screenshot = output + '/' + profile.name + '.png';
      await page.screenshot({ path: screenshot });
      cases.push({ browser: browserName, viewport: profile.name, passed: true,
        touchCancelCleared: true, focusLossCleared: true, keyboardCleared: true, screenshot });
      console.log(JSON.stringify(cases.at(-1)));
    } finally { await page.close(); }
  }
} finally { await browser.close(); }

writeFileSync(output + '/report.json', JSON.stringify({
  browser: browserName, passed: cases.length === 2, cases,
}, null, 2));
