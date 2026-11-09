#!/usr/bin/env node
// Prepare reviewable, version-bound poster candidates from REAL Chromium
// gameplay screenshots that already passed game-specific browser quality tests.
// This tool intentionally NEVER publishes media or writes into inventory.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { PNG } from 'pngjs';

const option = (key, fallback) => {
  const at = process.argv.indexOf('--' + key);
  return at >= 0 ? process.argv[at + 1] : fallback;
};
const out = resolve(option('out', '/tmp/preview-candidates'));
const shotsRoot = resolve(option('shots-root', '/tmp'));
const inventoryRoot = resolve(option('inventory', 'inventory'));
const sha256 = bytes => createHash('sha256').update(bytes).digest('hex');
const cases = [
  { dir: 'p3ngui', report: 'penguin-quality-chromium/report.json',
    screenshot: 'penguin-quality-chromium/iphone-portrait/restoring-aurora.png',
    story: 'Six-heart aurora progress (not a win or recorded clip)' },
  { dir: 'u46ex5', report: 'ribbon-quality-chromium/report.json',
    screenshot: 'ribbon-quality-chromium/iphone-portrait/hole3-stitched.png',
    story: 'Gameplay after a real sky-hole repair (not a win)' },
  { dir: '2uowg9', report: 'escape-quality-chromium/report.json',
    screenshot: 'escape-quality-chromium/iphone-portrait/falling-syringe-lane.png',
    story: 'Actual falling syringe in the three-lane chase (not a natural ending)' },
  { dir: '13ui1k', report: 'bobo-controls-chromium/report.json',
    screenshot: 'bobo-controls-chromium/before-short-tap.png',
    story: 'Centered source-drawing bear at live gameplay start; movement verified in the same run (not an observed completion)' },
];

mkdirSync(out, { recursive: true });
const result = [];
for (const item of cases) {
  const inventoryDir = join(inventoryRoot, item.dir);
  const manifest = JSON.parse(readFileSync(join(inventoryDir, 'manifest.json'), 'utf8'));
  const report = JSON.parse(readFileSync(join(shotsRoot, item.report), 'utf8'));
  assert.equal(report.passed, true, item.dir + ': quality report must pass before poster preparation');
  assert.ok(Number.isInteger(manifest.version) && manifest.version > 0,
    item.dir + ': manifest needs a published version');
  const screenshotPath = join(shotsRoot, item.screenshot);
  const poster = readFileSync(screenshotPath);
  assert.ok(poster.length > 100 && poster.length <= 2 * 1024 * 1024,
    item.dir + ': screenshot must meet preview storage byte budget');
  const png = PNG.sync.read(poster);
  assert.ok(png.width >= 240 && png.height >= 240 && png.width <= 4096 && png.height <= 4096,
    item.dir + ': preview image dimensions are out of bounds');
  if (item.dir === '13ui1k') {
    // The former after-tap poster captured Bobo at the extreme right
    // boundary, clipping most of the original crayon bear. Screenshots
    // reviewed alongside the source art show the before-tap gameplay
    // frame is centered while the same report still proves real tapping.
    assert.ok(Number.isFinite(report.before?.x) &&
      report.before.x >= png.width * 0.25 &&
      report.before.x <= png.width * 0.75 &&
      report.after?.x > report.before.x + 40,
    'Bobo poster must show a centered hero in a verified real-tap playtest');
  }
  // A solid-color/blank capture is NOT convincing gameplay evidence.
  const colors = new Set();
  const stride = Math.max(1, Math.floor(png.width * png.height / 4096));
  for (let pixel = 0; pixel < png.width * png.height; pixel += stride) {
    const i = pixel * 4;
    if (png.data[i + 3] < 16) continue;
    colors.add([png.data[i], png.data[i + 1], png.data[i + 2]].join(','));
  }
  assert.ok(colors.size > 24, item.dir + ': poster appears blank or monochrome');
  const ir = readFileSync(join(inventoryDir, 'game-ir.json'));
  const dest = join(out, item.dir);
  mkdirSync(dest, { recursive: true });
  const fixture = {
    version: manifest.version,
    ir_sha256: sha256(ir),
    poster_sha256: sha256(poster),
    source_run: process.env.GITHUB_RUN_ID
      ? 'https://github.com/GavinDu/html5-game-generator/actions/runs/' + process.env.GITHUB_RUN_ID
      : 'local browser quality run',
    source_commit: process.env.GITHUB_SHA || 'local',
    source_screenshot: item.screenshot,
    note: item.story + '. Candidate only: requires screenshot review before publishing.',
  };
  writeFileSync(join(dest, 'preview-fixture.json'), JSON.stringify(fixture, null, 2) + '\n');
  writeFileSync(join(dest, 'preview-poster.png.b64'), poster.toString('base64') + '\n');
  copyFileSync(screenshotPath, join(dest, 'proof.png'));
  result.push({ game_id: manifest.game_id, inventory_dir: item.dir,
    version: manifest.version, poster_sha256: fixture.poster_sha256,
    ir_sha256: fixture.ir_sha256, screenshot: item.screenshot,
    width: png.width, height: png.height, distinct_colors: colors.size,
    candidate_only: true });
}
writeFileSync(join(out, 'index.json'), JSON.stringify({
  status: 'AWAITING_SCREENSHOT_REVIEW',
  safe_to_publish_automatically: false,
  items: result,
}, null, 2) + '\n');
console.log(JSON.stringify({ status: 'AWAITING_SCREENSHOT_REVIEW', candidates: result }, null, 2));
