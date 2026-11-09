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
// Screenshot-based visual regression: a brown crayon pixel touching the
// right canvas edge means the enlarged Bear Prince was visibly cropped.
// The original child's PNG is not recolored or replaced for this check.
function brownPixelsAtRightEdge(bytes) {
  const p = PNG.sync.read(bytes);
  let clipped = 0;
  for (let y = Math.floor(p.height * 0.72); y < Math.floor(p.height * 0.94); y++) {
    for (let x = p.width - 3; x < p.width; x++) {
      const i = (y * p.width + x) * 4;
      const [r, g, b] = [p.data[i], p.data[i + 1], p.data[i + 2]];
      if (r >= 55 && r <= 190 && g >= 25 && g <= 150 &&
          b >= 15 && b <= 125 && r > g * 1.22 && g > b * 1.10) clipped++;
    }
  }
  return clipped;
}

// Gameplay screenshot regression: halo is a pale scene decoration behind
// Bobo. The separate white kitten to the RIGHT is part of the creator's
// original artwork and must not be mistaken for the halo. The earlier
// screenshot-only visual audit accidentally shifted the halo away from Bobo.
// After an actual right tap, the old spawn region must lose its pale fill.
function paleHaloPixelsAtSpawn(bytes) {
  const p = PNG.sync.read(bytes);
  assert.equal(p.width, 390, 'iPhone visual spawn guard assumes 390px capture');
  let n = 0;
  for (let y = 650; y < Math.min(p.height, 780); y++) {
    for (let x = 135; x < 235; x++) {
      const i = (y * p.width + x) * 4;
      const r = p.data[i], g = p.data[i + 1], b = p.data[i + 2];
      // Opaque pale ellipse over the authored sky blue; deliberately
      // excludes brown bear pixels and the kitten's separate circle.
      if (r >= 151 && r <= 158 && g >= 202 && g <= 208 &&
          b >= 226 && b <= 232) n++;
    }
  }
  return n;
}

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
  const oldSpawnPaleBefore = paleHaloPixelsAtSpawn(beforePNG);
  const oldSpawnPaleAfter = paleHaloPixelsAtSpawn(afterPNG);
  assert.ok(oldSpawnPaleBefore > 300,
    'before tap screenshot must actually show the pale halo at the original spawn');
  assert.ok(oldSpawnPaleAfter <= Math.max(25, oldSpawnPaleBefore * 0.15),
    'after real tap halo must not remain as a ghost at original spawn: ' +
    oldSpawnPaleBefore + ' before, ' + oldSpawnPaleAfter + ' after');
  const croppedBrownPixels = brownPixelsAtRightEdge(afterPNG);
  assert.ok(croppedBrownPixels <= 3,
    'real right-edge tap must not crop original brown source artwork (edge pixels=' + croppedBrownPixels + ')');
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
  // A feed cover selected from the first frame shows a mostly empty blue sky,
  // even though the real game progressively restores six rainbow bands.
  // Capture a *real* four-heart gameplay state as a candidate for later human
  // poster review; do not fake progress or automatically publish this image.
  let midplayEvidence = null;
  const midplayDeadline = Date.now() + 60000;
  while (Date.now() < midplayDeadline) {
    const state = await page.evaluate(() => window.__dqTest.snapshot());
    if (state.ended) break;
    if (state.objectiveProgress >= 4) {
      // Center the source crayon bear with another genuine touchscreen tap.
      // The screenshot should display both bear and kitten, not clip the bear.
      await page.touchscreen.tap(rect.x + rect.width * 0.5,
        rect.y + rect.height * 0.6);
      await page.waitForFunction(() => {
        const s = window.__dqTest.snapshot();
        return !s.ended && Math.abs(s.player.x - 195) <= 28;
      }, null, { timeout: 4000 });
      const progress = await page.evaluate(() => window.__dqTest.snapshot());
      assert.ok(progress.objectiveProgress >= 4 && progress.objectiveProgress < 12,
        'Bobo poster candidate must show mid-play, not an untouched start or end screen');
      for (const id of ['band0', 'band1']) {
        assert.ok(progress.entities.find(e => e.id === id)?.alpha > .9,
          'first two rainbow bands must be visibly restored in the real mid-play screenshot');
      }
      const shot = await page.screenshot({ path: shots + '/midplay-restored-rainbow.png' });
      assert.ok(brownPixelsAtRightEdge(shot) <= 3,
        'centered Bobo poster must not clip the original source crayon bear');
      midplayEvidence = {
        progress: progress.objectiveProgress,
        score: progress.score,
        playerX: progress.player.x,
        source: 'guided real touch input',
        screenshot: 'midplay-restored-rainbow.png',
      };
      break;
    }
    const targets = state.entities
      .filter(e => e.kind === 'pickup' &&
        (e.id === 'heart' || e.id === 'heart_blue') &&
        e.visible && Number.isFinite(e.x) && Number.isFinite(e.y) &&
        e.y < state.player.y + 20)
      .sort((a, b) => b.y - a.y);
    if (targets[0]) {
      await page.touchscreen.tap(rect.x + (targets[0].x / 390) * rect.width,
        rect.y + rect.height * 0.6);
    }
    await page.waitForTimeout(110);
  }
  assert.ok(midplayEvidence, 'four real regular-heart catches must produce reviewable mid-play poster evidence');

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
    const tabletCanvas = await tablet.locator('#stage canvas').screenshot({
      path: shots + '/ipad-after-short-tap-stage.png',
    });
    assert.ok(brownPixelsAtRightEdge(tabletCanvas) <= 3,
      'iPad logical game stage must keep the original crayon bear fully inside its right boundary');
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
    croppedBrownPixels, visualArtScale: playerIR.render.scale,
    haloFollowsPlayer: true,
    midplayEvidence,
    screenshotOldSpawnPalePixelsBefore: oldSpawnPaleBefore,
    screenshotOldSpawnPalePixelsAfter: oldSpawnPaleAfter,
    physicalSize: playerIR.size,
    screenshots: ['before-short-tap.png', 'after-short-tap.png',
      'ipad-before-short-tap.png', 'ipad-after-short-tap.png',
      'ipad-after-short-tap-stage.png', 'midplay-restored-rainbow.png',
      'missed-heart-before-recycle.png', 'missed-heart-after-recycle.png']
  };
  writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report));
} finally { await browser.close(); }
