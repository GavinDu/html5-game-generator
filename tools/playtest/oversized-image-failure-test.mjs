#!/usr/bin/env node
// Fail-closed security probe for oversized AI-authored image transfers.
// Use the real package and Pixi/WebKit runtime; only the image asset
// response body is replaced. Preserve the original IR and package hash.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const output = arg('shots', '/tmp/runtime-image-budget');
const browser = await (browserName === 'webkit' ? webkit : chromium).launch({ headless: true });
const id = 'penguin-heartlight-rescue';
mkdirSync(output, { recursive: true });

const packageResponse = await fetch(base + '/v1/games/' + id + '/package');
assert.equal(packageResponse.status, 200, 'real public Game Package must be deliverable');
const pkg = await packageResponse.json();
const imageAsset = pkg.assets.find(a => a.kind === 'image' && a.url);
assert.ok(imageAsset, 'inventory fixture has no required image');
const url = new URL(imageAsset.url, base).href;
const hugeBytes = Buffer.alloc(8 * 1024 * 1024 + 1, 0);
const result = { browser: browserName, asset: imageAsset.id, rejected: false };
const page = await browser.newPage({ viewport: { width: 390, height: 844 }, hasTouch: true });
try {
  await page.route(url, async route => {
    await route.fulfill({ status: 200, contentType: 'image/png', body: hugeBytes });
  });
  await page.goto(base + '/player?game=' + id + '&presentation=play&probe=1', {
    waitUntil: 'load', timeout: 30000,
  });
  await page.waitForFunction(() => document.querySelector('#status')?.textContent?.includes('maximum compressed byte budget'), null, { timeout: 20000 });
  const status = await page.locator('#status').textContent();
  assert.match(status, /Failed to load: required image asset/);
  assert.match(status, /exceeds maximum compressed byte budget/);
  assert.equal(await page.evaluate(() => !!window.__dqTest?.snapshot), false,
    'oversized required image cannot reach a playable state');
  const screenshot = output + '/' + browserName + '-oversized-image-rejected.png';
  await page.screenshot({ path: screenshot, fullPage: true });
  result.rejected = true;
  result.reason = 'compressed-byte-limit-before-decode';
  result.screenshot = screenshot;
  console.log(JSON.stringify(result));
} finally {
  await page.close();
  await browser.close();
}
writeFileSync(output + '/report.json', JSON.stringify(result, null, 2));
