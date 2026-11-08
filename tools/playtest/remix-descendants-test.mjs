#!/usr/bin/env node
// Play two generations of every inventory game's canonical remix through the
// REAL HTTP generation API and the PixiJS browser player. The API-only matrix
// proves lineage, but cannot detect a child that looks valid and will not play.
// CI launches a separate temporary backend with an explicitly local-only test
// identity. No generated child is ever published to a production feed.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
import { PNG } from 'pngjs';

const arg = (key, fallback) => {
  const p = process.argv.indexOf('--' + key);
  return p >= 0 ? process.argv[p + 1] : fallback;
};
const base = arg('base', 'http://127.0.0.1:18102').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const out = arg('shots', '/tmp/remix-descendants');
const identity = 'remix-browser-ci-creator';
if (!['chromium', 'webkit'].includes(browserName)) throw Error('unsupported browser');
mkdirSync(out, { recursive: true });
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const sources = [
  'sample-rainbow-bear',
  'sample-ribbon-sky',
  'penguin-heartlight-rescue',
  'escape-from-injection',
];
const results = [];

const request = async (path, method = 'GET', body) => {
  const response = await fetch(base + path, {
    method,
    headers: {
      'X-User-ID': identity,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    ...(body ? { body: JSON.stringify(body) } : {}),
    signal: AbortSignal.timeout(20000),
  });
  const text = await response.text();
  if (!response.ok && response.status !== 202) {
    throw Error(method + ' ' + path + ': HTTP ' + response.status + ' ' + text.slice(0, 500));
  }
  return JSON.parse(text);
};

const awaitJob = async (jobID, expected) => {
  const deadline = Date.now() + 35000;
  while (Date.now() < deadline) {
    const job = await request('/v1/jobs/' + encodeURIComponent(jobID));
    if (job.status === expected) return job;
    if (job.status === 'failed') throw Error('job ' + jobID + ': ' + job.error);
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  throw Error('generation job ' + jobID + ' timed out before ' + expected);
};

const delta = (a, b) => {
  const x = PNG.sync.read(a), y = PNG.sync.read(b);
  assert.equal(x.width, y.width);
  assert.equal(x.height, y.height);
  let changed = 0;
  for (let i = 0; i < x.data.length; i += 4) {
    if (Math.abs(x.data[i] - y.data[i]) > 20 ||
        Math.abs(x.data[i + 1] - y.data[i + 1]) > 20 ||
        Math.abs(x.data[i + 2] - y.data[i + 2]) > 20) changed++;
  }
  return changed / (x.width * x.height);
};

const playChild = async (gameID, folder) => {
  mkdirSync(folder, { recursive: true });
  const errors = [];
  const context = await browser.newContext({
    viewport: { width: 390, height: 844 },
    hasTouch: true, deviceScaleFactor: 1,
    extraHTTPHeaders: { 'X-User-ID': identity },
  });
  const page = await context.newPage();
  page.on('pageerror', error => errors.push(String(error)));
  try {
    await page.goto(base + '/player?game=' + encodeURIComponent(gameID) +
      '&presentation=play&probe=1', { waitUntil: 'load', timeout: 30000 });
    await page.waitForFunction(() => {
      const s = window.__dqTest?.snapshot?.();
      return s && s.t > 0.15 && s.player && !s.ended;
    }, null, { timeout: 20000 });
    const before = await page.evaluate(() => window.__dqTest.snapshot());
    const canvas = await page.locator('#stage canvas').boundingBox();
    assert.ok(canvas && canvas.width > 200 && canvas.height > 200, 'no playable canvas');
    const screenshotBefore = await page.screenshot({ path: folder + '/before.png' });
    // Drive a real touchscreen pointer; no mutable test shortcuts or injected
    // success flags. All four source questionnaires have tap/lane_tap movement.
    await page.touchscreen.tap(canvas.x + canvas.width * 0.85,
      canvas.y + canvas.height * 0.65);
    await page.waitForFunction(x => {
      const s = window.__dqTest?.snapshot?.();
      return s?.player && (s.player.x > x + 12 || s.player.laneIndex === 2);
    }, before.player.x, { timeout: 8000 });
    const after = await page.evaluate(() => window.__dqTest.snapshot());
    assert.ok(after.t > before.t, 'gameplay simulation did not advance');
    assert.ok(after.player.x > before.player.x + 12 ||
      after.player.laneIndex === 2, 'real tap did not move the controlled hero');
    const screenshotAfter = await page.screenshot({ path: folder + '/after-real-tap.png' });
    const visualDelta = delta(screenshotBefore, screenshotAfter);
    assert.ok(visualDelta > 0.005, 'no meaningful rendered change after gameplay');
    assert.deepEqual(errors, [], 'uncaught browser error on generated child');
    return {
      passed: true, gameID, beforeScore: before.score, afterScore: after.score,
      initialX: before.player.x, afterX: after.player.x,
      simTime: after.t, visualDelta, screenshots: ['before.png', 'after-real-tap.png'],
    };
  } catch (error) {
    await page.screenshot({ path: folder + '/failed.png' }).catch(() => {});
    return { passed: false, gameID, error: String(error), pageErrors: errors };
  } finally {
    await context.close();
  }
};

try {
  for (const parent of sources) {
    const family = { parent, generations: [] };
    let current = parent;
    for (const generation of [1, 2]) {
      const created = await request('/v1/games', 'POST', {
        parent_game_id: current, intent: 'Preserve source mechanics through a creative remix',
      });
      const id = created.game?.game_id, jobID = created.job?.job_id;
      assert.ok(id && jobID, 'generation API omitted the child ID or job ID');
      await awaitJob(jobID, 'awaiting_confirm');
      await request('/v1/games/' + encodeURIComponent(id) + '/confirm', 'POST', {});
      await awaitJob(jobID, 'done');
      const pkg = await request('/v1/games/' + encodeURIComponent(id) + '/package');
      assert.equal(pkg.ir?.metadata?.parentGameId, current,
        'child package lost immediate parent');
      const result = await playChild(id, out + '/' + parent + '-gen' + generation);
      family.generations.push({ generation, parentID: current, ...result });
      current = id;
      if (!result.passed) throw Error('browser gameplay failed for ' + parent + ' generation ' + generation +
        ': ' + result.error);
    }
    results.push(family);
    console.log(JSON.stringify(family));
  }
} catch (error) {
  console.error('remix descendants browser gate: ' + (error.stack || error));
  process.exitCode = 1;
} finally {
  await browser.close();
  writeFileSync(out + '/report.json', JSON.stringify({
    browser: browserName,
    passed: !process.exitCode && results.length === sources.length &&
      results.every(f => f.generations.length === 2 && f.generations.every(g => g.passed)),
    results,
  }, null, 2));
}
