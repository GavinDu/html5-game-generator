#!/usr/bin/env node
// Actual short touch contacts must move the catcher; missed hearts must return.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
import { PNG } from 'pngjs';
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
  // Asset fidelity is a dual contract: the creator's original crayon art
  // must be rendered at a readable scale without expanding the physical
  // catcher into a much larger invisible hitbox.
  const pkgRes = await page.request.get(base + '/v1/games/sample-rainbow-bear/package');
  assert.equal(pkgRes.status(), 200);
  const pkg = await pkgRes.json();
  const playerIR = pkg.ir.scenes.flatMap(s => s.entities).find(e => e.id === 'player');
  assert.ok(playerIR, 'canonical Bobo must exist');
  assert.deepEqual(playerIR.size, [92, 104], 'keep authored collision footprint');
  assert.deepEqual(playerIR.render?.scale, [1.28, 1.28], 'increase only visible crayon art scale');
  const haloIR = pkg.ir.scenes.flatMap(s => s.entities).find(e => e.id === 'halo-bear');
  assert.deepEqual(haloIR?.behavior, { type: 'follow', target: 'player' },
    'the bear ground halo must follow the author\'s real player sprite as visual-only decoration');
  const assertHaloAttached = (snap, name) => {
    const halo = snap.entities.find(e => e.id === 'halo-bear');
    assert.ok(halo, name + ': halo decoration missing');
    assert.ok(Math.abs(halo.x - snap.player.x) <= 2,
      name + ': original ground halo must not stay behind while player moves');
    assert.ok(Math.abs(halo.y - snap.player.y) <= 2,
      name + ': halo must remain vertically aligned with the original bear');
  };
  const before = await page.evaluate(() => window.__dqTest.snapshot());
  assertHaloAttached(before, 'iPhone initial');
  const beforePNG = await page.screenshot({ path: shots + '/before-short-tap.png' });
  const rect = await page.locator('#stage canvas').boundingBox();
  // A single short tap, with no held-contact fallback or runtime state writes.
  await page.touchscreen.tap(rect.x + rect.width * 0.85, rect.y + rect.height * 0.6);
  await page.waitForFunction(x => window.__dqTest.snapshot().player.x > x + 40, before.player.x, { timeout: 3000 });
  const moved = await page.evaluate(() => window.__dqTest.snapshot());
  assertHaloAttached(moved, 'iPhone real right tap');
  assert.ok(Math.abs(moved.player.y - before.player.y) < 2, 'catcher must stay in the horizontal lane');
  const afterPNG = await page.screenshot({ path: shots + '/after-short-tap.png' });
  const changed = (a, b) => {
    const p = PNG.sync.read(a), q = PNG.sync.read(b);
    assert.equal(p.width, q.width);
    assert.equal(p.height, q.height);
    let diff = 0;
    for (let i = 0; i < p.data.length; i += 4) {
      if (Math.abs(p.data[i] - q.data[i]) > 20 ||
          Math.abs(p.data[i + 1] - q.data[i + 1]) > 20 ||
          Math.abs(p.data[i + 2] - q.data[i + 2]) > 20) diff++;
    }
    return diff / (p.width * p.height);
  };
  const visualDelta = changed(beforePNG, afterPNG);
  assert.ok(visualDelta > 0.005, 'real tap must visibly move the original crayon hero');
  // Leave the blue heart alone on the other side, observe it fall, then return
  // to the top. Horizontal sway must not keep a missed heart stuck forever.
  await page.waitForFunction(() => window.__dqTest.snapshot().entities.some(e => e.id === 'heart_blue' && e.y > 770), null, { timeout: 10000 });
  const low = await page.evaluate(() => window.__dqTest.snapshot().entities.find(e => e.id === 'heart_blue'));
  await page.screenshot({ path: shots + '/missed-heart-before-recycle.png' });
  await page.waitForFunction(() => window.__dqTest.snapshot().entities.some(e => e.id === 'heart_blue' && e.y < 100), null, { timeout: 12000 });
  const recycled = await page.evaluate(() => window.__dqTest.snapshot().entities.find(e => e.id === 'heart_blue'));
  await page.screenshot({ path: shots + '/missed-heart-after-recycle.png' });
  // The responsive web player also runs on an iPad viewport. Keep the
  // proof screenshot: a 1.28x visual scale must not produce a cut-off hero
  // or absorb touches in the tablet's centered portrait game stage.
  const tablet = await browser.newPage({ viewport: { width: 820, height: 1180 }, hasTouch: true });
  let tabletDelta;
  try {
    await tablet.goto(base + '/player?game=sample-rainbow-bear&presentation=play&probe=1');
    await tablet.waitForFunction(() => window.__dqTest?.snapshot?.().t > 0.2);
    const tabletBefore = await tablet.evaluate(() => window.__dqTest.snapshot());
    const tabletBeforePNG = await tablet.screenshot({ path: shots + '/ipad-before-short-tap.png' });
    const tabletRect = await tablet.locator('#stage canvas').boundingBox();
    assert.ok(tabletRect, 'iPad game canvas missing');
    await tablet.touchscreen.tap(tabletRect.x + tabletRect.width * 0.85,
      tabletRect.y + tabletRect.height * 0.6);
    await tablet.waitForFunction(x => window.__dqTest.snapshot().player.x > x + 40,
      tabletBefore.player.x, { timeout: 4000 });
    const tabletAfter = await tablet.evaluate(() => window.__dqTest.snapshot());
    assertHaloAttached(tabletAfter, 'iPad real right tap');
    assert.ok(Math.abs(tabletAfter.player.y - tabletBefore.player.y) < 2,
      'iPad Bobo must stay in the authored catcher lane');
    const tabletAfterPNG = await tablet.screenshot({ path: shots + '/ipad-after-short-tap.png' });
    tabletDelta = changed(tabletBeforePNG, tabletAfterPNG);
    assert.ok(tabletDelta > 0.005, 'iPad gameplay screenshots must visibly change after real tap');
  } finally {
    await tablet.close();
  }
  const report = {
    passed: true,
    gesture: 'real touchscreen tap',
    before: before.player, after: moved.player, low, recycled,
    phoneVisualDelta: visualDelta,
    tabletVisualDelta: tabletDelta,
    visualArtScale: playerIR.render.scale,
    haloFollowsPlayer: true,
    physicalSize: playerIR.size,
    screenshots: ['before-short-tap.png', 'after-short-tap.png',
      'ipad-before-short-tap.png', 'ipad-after-short-tap.png',
      'missed-heart-before-recycle.png', 'missed-heart-after-recycle.png']
  };
  writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report));
} finally { await browser.close(); }
