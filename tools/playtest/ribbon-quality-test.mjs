#!/usr/bin/env node
// Browser acceptance: only six distinct sky-hole contacts can finish Ribbon.
// The bot drives real touchscreen taps, and uses screenshots as both inputs
// (measurable visual differences) and retained review evidence.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { PNG } from 'pngjs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const index = process.argv.indexOf('--' + name);
  return index < 0 ? fallback : process.argv[index + 1];
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/ribbon-quality');
const engine = browserName === 'webkit' ? webkit : chromium;
const profiles = [
  { name: 'iphone-portrait', width: 390, height: 844 },
  { name: 'ipad-portrait', width: 820, height: 1180 },
];
const HOLE_IDS = Array.from({ length: 6 }, (_, i) => 'hole' + (i + 1));
mkdirSync(shots, { recursive: true });

function changedPixelRatio(a, b) {
  const first = PNG.sync.read(a), second = PNG.sync.read(b);
  assert.equal(first.width, second.width);
  assert.equal(first.height, second.height);
  let changed = 0, count = 0;
  for (let y = 0; y < first.height; y += 5) {
    for (let x = 0; x < first.width; x += 5) {
      const index = (y * first.width + x) * 4;
      const distance = Math.abs(first.data[index] - second.data[index]) +
        Math.abs(first.data[index + 1] - second.data[index + 1]) +
        Math.abs(first.data[index + 2] - second.data[index + 2]);
      if (distance > 42) changed++;
      count++;
    }
  }
  return changed / Math.max(1, count);
}

function stitched(snapshot) {
  return Number(snapshot.variables?.holes_stitched ?? 0);
}
function repaired(snapshot) {
  return HOLE_IDS.filter(id => snapshot.entities.some(e =>
    e.id === id && e.assetId === 'hole_closed'));
}

const browser = await engine.launch({ headless: true });
const results = [];
try {
  const packageResponse = await fetch(base + '/v1/games/sample-ribbon-sky/package');
  assert.ok(packageResponse.ok, 'canonical Ribbon package must be available');
  const pkg = await packageResponse.json();
  assert.equal(pkg.ir.experience.objective.progressVariable, 'holes_stitched');
  const allHoles = pkg.ir.scenes[0].entities.filter(e => HOLE_IDS.includes(e.id));
  assert.equal(allHoles.length, 6, 'exactly six unique holes must be authored');
  assert.ok(allHoles.every(e => e.kind === 'pickup' && e.physics?.sensor && e.properties?.closeOnCollect));
  assert.ok(pkg.ir.rules.filter(rule => rule.actions.some(a => a.type === 'end_game' && a.value === true))
    .every(rule => rule.when.event === 'variable_changed' &&
      rule.conditions?.some(c => c.variable === 'holes_stitched' && c.operator === 'eq' && c.value === 6)),
    'bonus score may never be a win condition');

  for (const profile of profiles) {
    const dir = shots + '/' + profile.name;
    mkdirSync(dir, { recursive: true });
    const page = await browser.newPage({
      viewport: { width: profile.width, height: profile.height },
      hasTouch: true, deviceScaleFactor: 1,
    });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    const snap = () => page.evaluate(() => window.__dqTest.snapshot());
    const capture = name => page.screenshot({ path: dir + '/' + name + '.png' });
    try {
      await page.goto(base + '/player?game=sample-ribbon-sky&presentation=play&probe=1');
      await page.waitForFunction(() => window.__dqTest?.snapshot?.().t > 0.2, null, { timeout: 20000 });
      const before = await capture('before-play');
      const initial = await snap();
      assert.equal(initial.world.w, 390);
      assert.equal(initial.world.h, 844);

      // Collect ordinary falling shards without steering into the six holes.
      // A high bonus score must never substitute for repairing the sky.
      const bonusDeadline = Date.now() + 25000;
      let bonus = initial;
      while (Date.now() < bonusDeadline) {
        bonus = await snap();
        if (bonus.score >= 10 || bonus.ended) break;
        await page.waitForTimeout(300);
      }
      assert.ok(bonus.score >= 10, 'visible bonus shards should yield at least ten points');
      assert.equal(bonus.ended, false, 'bonus score alone must not finish the game');
      assert.ok(stitched(bonus) < 6, 'bonus-only play must leave unrepaired holes');
      const bonusImage = await capture('bonus-score-cannot-win');

      const tap = async (x, y) => {
        const box = await page.locator('#stage canvas').boundingBox();
        assert.ok(box && box.width > 100 && box.height > 100, 'game canvas must remain visible');
        await page.touchscreen.tap(box.x + (x / 390) * box.width,
          box.y + (y / 844) * box.height);
      };
      let duplicateChecked = false;
      const individuallyRepaired = [];
      for (const id of HOLE_IDS) {
        let current = await snap();
        let hole = current.entities.find(e => e.id === id);
        assert.ok(hole, id + ' must remain visible in the game world');
        if (hole.assetId === 'hole_closed') continue;
        const oldProgress = stitched(current);
        await tap(hole.x, hole.y);
        await page.waitForFunction(holeID => {
          const state = window.__dqTest?.snapshot();
          return state?.entities.some(e => e.id === holeID && e.assetId === 'hole_closed');
        }, id, { timeout: 7000 });
        current = await snap();
        assert.equal(stitched(current), repaired(current).length,
          'objective progress must count unique closed patches, not score');
        assert.ok(stitched(current) >= oldProgress + 1, 'a new hole should increase progress');
        individuallyRepaired.push(id);
        await capture(id + '-stitched');

        if (!duplicateChecked && !current.ended) {
          const progressOnce = stitched(current);
          await tap(hole.x, hole.y);
          await page.waitForTimeout(550);
          assert.equal(stitched(await snap()), progressOnce,
            'repeated taps over the same repaired hole must not add progress');
          duplicateChecked = true;
        }
      }
      const completed = await snap();
      assert.deepEqual(repaired(completed).sort(), [...HOLE_IDS].sort());
      assert.equal(stitched(completed), 6);
      assert.equal(completed.won, true, 'six distinct stitched holes must end in success');
      assert.equal(completed.ended, true);
      assert.ok(changedPixelRatio(before, bonusImage) > 0.002,
        'real screenshots must show a measurable gameplay change');
      const winImage = await capture('six-unique-holes-win');
      const visualDelta = changedPixelRatio(bonusImage, winImage);
      assert.ok(visualDelta > 0.005,
        'screenshots must visually differ after the six-hole repair');
      assert.equal(errors.length, 0, 'runtime must not emit page errors');

      const marker = 'ribbon-' + profile.name;
      await page.evaluate(value => { window.__ribbonRestartMarker = value; }, marker);
      await page.locator('#shell-again-btn').click();
      await page.waitForFunction(() => {
        const s = window.__dqTest?.snapshot();
        return s && !s.ended && s.t < 5 && (s.variables?.holes_stitched ?? 0) < 2;
      }, null, { timeout: 8000 });
      assert.equal(await page.evaluate(() => window.__ribbonRestartMarker), marker,
        'Play again must restart in place, without reloading');
      await capture('restart');

      const result = {
        profile: profile.name, browser: browserName, passed: true,
        bonusScoreBeforeRepair: bonus.score,
        repairsBeforeSteering: stitched(bonus),
        individuallyRepaired, finalProgress: stitched(completed),
        closedPatches: repaired(completed), won: completed.won,
        duplicateChecked, visualDelta, restartInPlace: true,
        screenshots: ['before-play', 'bonus-score-cannot-win',
          ...individuallyRepaired.map(id => id + '-stitched'), 'six-unique-holes-win', 'restart'],
      };
      results.push(result);
      writeFileSync(dir + '/report.json', JSON.stringify(result, null, 2));
      console.log(JSON.stringify(result));
    } finally {
      await page.close();
    }
  }
  writeFileSync(shots + '/report.json', JSON.stringify({ passed: true, results }, null, 2));
} finally {
  await browser.close();
}
