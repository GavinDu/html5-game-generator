#!/usr/bin/env node
// Epic #202 / P0 #315: real production Pixi runtime/DOM evidence for an
// input trace that exceeds Go's 2,000-event authoritative replay budget.
// Only browser-native key events are dispatched: no mutation probe hooks,
// runtime-private functions, test-only score setters or fake verified result.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (key, fallback) => {
  const i = process.argv.indexOf('--' + key);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/replay-overflow');
mkdirSync(shots, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const report = [];

try {
  for (const device of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: device.width, height: device.height }, hasTouch: true,
    });
    const errors = [];
    const runSubmissions = [];
    page.on('pageerror', error => errors.push(String(error)));
    page.on('request', request => {
      if (request.method() === 'POST' && /\/v1\/games\/[^/]+\/runs(?:\?|$)/.test(request.url())) {
        runSubmissions.push(request.url());
      }
    });
    try {
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1',
        { timeout: 30000 });
      await page.waitForFunction(() => Boolean(window.__dqTest?.snapshot?.()?.player),
        null, { timeout: 30000 });
      await page.evaluate(() => window.__dqBeginPlay?.());
      await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.started === true,
        null, { timeout: 20000 });

      // A sequence of browser keyup events exercises the actual production
      // keyboard handler and recorder but does not enqueue game commands.
      // A synchronous task avoids a semantic tick between event delivery.
      // This isolates replay-evidence overflow from the 128-command queue
      // overflow test, which legitimately fails the active simulation.
      const dispatched = await page.evaluate(() => {
        const count = 2001;
        for (let i = 0; i < count; i++) {
          window.dispatchEvent(new KeyboardEvent('keyup', {
            bubbles: true, key: 'ArrowLeft',
          }));
        }
        return count;
      });
      assert.equal(dispatched, 2001);
      await page.waitForFunction(() => window.__dqProbe?.replayTraceComplete === false &&
        window.__dqProbe?.inputs === 2000, null, { timeout: 10000 });
      const evidence = await page.evaluate(() => ({
        traceComplete: window.__dqProbe.replayTraceComplete,
        eventsRetained: window.__dqProbe.inputs,
        diagnosticsComplete: window.__dqProbe.deadInputDiagnosticsComplete,
        deadInputRate: window.__dqProbe.deadInputRate,
        lastRunId: window.__dqProbe.lastRunId,
        failed: window.__dqTest.snapshot().failed,
      }));
      assert.equal(evidence.traceComplete, false);
      assert.equal(evidence.eventsRetained, 2000, 'must keep exact bounded replay prefix');
      assert.equal(evidence.diagnosticsComplete, false, '257th sample invalidates a bounded diagnostic');
      assert.equal(evidence.deadInputRate, null, 'incomplete diagnostics cannot yield a confident rate');
      assert.equal(evidence.lastRunId, null, 'no verified run was produced during the overflow check');
      assert.equal(runSubmissions.length, 0, 'incomplete trace cannot submit a leaderboard run');
      assert.equal(evidence.failed, false, 'trace overflow must not crash game simulation');
      assert.deepEqual(errors, [], 'no uncaught browser exception from overflow');

      const screenshot = shots + '/' + browserName + '-' + device.name + '.png';
      await page.screenshot({ path: screenshot, fullPage: true });
      report.push({ browser: browserName, device: device.name, dispatched,
        ...evidence, runSubmissions: runSubmissions.length, screenshot });
      console.log(JSON.stringify(report.at(-1)));
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}

writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, passed: report.length === 2, cases: report,
}, null, 2));
assert.equal(report.length, 2);
