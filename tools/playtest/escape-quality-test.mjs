#!/usr/bin/env node
// Physical touchscreen regression for the Escape questionnaire's three lanes.
// Screenshot pairs are retained as inputs and output evidence in GitHub Actions.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
import { PNG } from 'pngjs';

const option = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i < 0 ? fallback : process.argv[i + 1];
};
const base = option('base', 'http://localhost:18099').replace(/[/]$/, '');
const browserName = option('browser', 'chromium');
const shots = option('shots', '/tmp/escape-quality');
const engine = browserName === 'webkit' ? webkit : chromium;
mkdirSync(shots, { recursive: true });

function changeRatio(a, b) {
  const first = PNG.sync.read(a), second = PNG.sync.read(b);
  assert.equal(first.width, second.width);
  assert.equal(first.height, second.height);
  let changed = 0;
  for (let i = 0; i < first.data.length; i += 16) {
    const diff = Math.abs(first.data[i] - second.data[i]) +
      Math.abs(first.data[i + 1] - second.data[i + 1]) +
      Math.abs(first.data[i + 2] - second.data[i + 2]);
    if (diff > 42) changed++;
  }
  return changed / Math.ceil(first.data.length / 16);
}

const browser = await engine.launch({ headless: true });
const results = [];
try {
  for (const profile of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 }
  ]) {
    const dir = shots + '/' + profile.name;
    mkdirSync(dir, { recursive: true });
    const page = await browser.newPage({
      viewport: { width: profile.width, height: profile.height },
      hasTouch: true, deviceScaleFactor: 1,
    });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    const snap = () => page.evaluate(() => window.__dqTest?.snapshot());
    const capture = name => page.screenshot({ path: dir + '/' + name + '.png' });
    try {
      await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1');
      await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.t > 0.25, null, { timeout: 20000 });
      const before = await capture('before');
      const initial = await snap();
      assert.equal(initial.world.w, 390);
      assert.equal(initial.player?.laneIndex, 1, 'Escape must start in middle lane');
      const startY = initial.player.y;
      // Two readable falling syringe hazards, never a three-lane wall.
      const syringes = initial.entities.filter(e => /^syringe(?:02)?$/.test(e.def));
      assert.equal(syringes.length, 2, 'two physical falling syringes must be in the game');
      assert.deepEqual(syringes.map(e => e.x).sort((a, b) => a - b), [97.5, 292.5],
        'authored hazard lanes must start distinct, leaving middle safe');
      const firstChaser = initial.entities.find(e => e.def === 'chaser');
      assert.ok(firstChaser, 'the authored chaser must exist');
      assert.ok(Math.abs(firstChaser.y - startY - initial.variables.chaser_gap) < 2,
        'the rendered chaser must trail by the actual gap, not overlap the hero');
      assert.ok(initial.variables.chaser_gap >= 220 && initial.variables.chaser_gap <= 250,
        'the gap begins at 220 and recovers toward 250');
      const box = await page.locator('#stage canvas').boundingBox();
      assert.ok(box && box.width > 100 && box.height > 100);
      const tap = (x, y = 550) => page.touchscreen.tap(
        box.x + x / 390 * box.width, box.y + y / 844 * box.height);
      // Real taps, not injected game-state calls. Each tap moves exactly one lane.
      for (const [x, expectedLane] of [
        [375, 2], [375, 2], [15, 1], [15, 0], [15, 0], [375, 1]
      ]) {
        await tap(x);
        await page.waitForFunction(({ lane, x }) => {
          const player = window.__dqTest.snapshot().player;
          return player?.laneIndex === lane && Math.abs(player.x - x) < 12;
        }, { lane: expectedLane, x: [97.5, 195, 292.5][expectedLane] }, { timeout: 5000 });
        const current = await snap();
        assert.ok(Math.abs(current.player.x - [97.5, 195, 292.5][expectedLane]) < 12,
          'tap must snap to an authored lane center, not its XY destination');
        assert.ok(Math.abs(current.player.y - startY) < 2, 'runner height must remain fixed');
        assert.equal(current.ended, false, 'normal lane input must not end game');
      }
      const after = await capture('after-six-real-taps');
      const visualDelta = changeRatio(before, after);
      assert.ok(visualDelta > 0.005, 'before/after screenshots must differ');
      await page.waitForFunction(() => window.__dqTest.snapshot().score >= 20,
        null, { timeout: 10000 });
      const progressed = await snap();
      assert.ok(progressed.score >= 20, 'running adds 10 score points each second');
      assert.equal(progressed.won, false, 'bonus or running score may not invent an escape win');
      assert.equal(progressed.ended, false, 'the authored ending is caught, never a score threshold');
      assert.ok(progressed.variables.chaser_gap <= 250, 'lead recovery must be capped');
      assert.equal(errors.length, 0, 'no page runtime exceptions');
      await capture('score-without-false-victory');
      // These screenshots and assertions verify a real running hazard,
      // not an injected collider fixture. Missed syringes must fall off
      // the screen and respawn at one of the three lane centers.
      await page.waitForFunction(() => {
        const s = window.__dqTest?.snapshot?.();
        return s && !s.ended && s.entities.some(e => e.def === 'syringe' && e.y > 740);
      }, null, { timeout: 15000 });
      const falling = await snap();
      assert.ok([97.5, 195, 292.5].some(x =>
        Math.abs(falling.entities.find(e => e.def === 'syringe').x - x) < 1.5),
      'falling syringe must occupy a legal lane, not home toward the player');
      await capture('falling-syringe-lane');
      await page.waitForFunction(() => {
        const s = window.__dqTest?.snapshot?.();
        return s && !s.ended && s.entities.some(e => e.def === 'syringe' && e.y < 0);
      }, null, { timeout: 15000 });
      const recycled = await snap();
      assert.ok([97.5, 195, 292.5].some(x =>
        Math.abs(recycled.entities.find(e => e.def === 'syringe').x - x) < 1.5),
      'recycled syringe must respawn on one of three lanes');
      await capture('recycled-syringe-lane');
      const report = {
        browser: browserName, viewport: profile.name, passed: true,
        fallingSyringeVerified: true, recycledSyringeVerified: true,
        startingGap: initial.variables.chaser_gap, endingGap: progressed.variables.chaser_gap,
        initialLane: 1, finalLane: progressed.player.laneIndex,
        initialRunnerY: startY, score: progressed.score,
        falseVictory: progressed.ended, visualDelta,
        screenshots: ['before', 'after-six-real-taps', 'score-without-false-victory',
          'falling-syringe-lane', 'recycled-syringe-lane']
      };
      results.push(report);
      writeFileSync(dir + '/report.json', JSON.stringify(report, null, 2));
      // Controlled collision fixture: the canonical rule engine and Rapier
      // remain real, but start one syringe at the player's collider and use
      // a 70-unit lead so a single *physical collision* exercises the caught
      // result card and in-place restart without spending 20 minutes in CI.
      // This verifies terminal plumbing, NOT natural long-run balance.
      const terminal = await browser.newPage({
        viewport: { width: profile.width, height: profile.height },
        hasTouch: true, deviceScaleFactor: 1,
      });
      let fixtureFetches = 0;
      try {
        await terminal.route('**/v1/games/escape-from-injection/package', async route => {
          const source = await route.fetch();
          const payload = await source.json();
          if (fixtureFetches++ === 0) {
            payload.ir.experience.chaserGap.start = 70;
            const syringe = payload.ir.scenes.flatMap(scene => scene.entities)
              .find(ent => ent.id === 'syringe');
            assert.ok(syringe, 'controlled fixture must retain the real syringe rule');
            syringe.position = [195, 660];
            syringe.behavior = undefined;
            syringe.physics.body = 'dynamic';
            syringe.physics.sensor = true;
            syringe.physics.gravityScale = 0;
          }
          await route.fulfill({ response: source, json: payload });
        });
        await terminal.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1');
        await terminal.waitForFunction(() => window.__dqTest?.snapshot?.()?.ended,
          null, { timeout: 15000 });
        const caught = await terminal.evaluate(() => window.__dqTest.snapshot());
        assert.equal(caught.won, false, 'physical syringe contact must lead to caught, never a win');
        assert.ok(caught.variables.chaser_gap <= 55, 'collision must deplete the gap');
        assert.equal(await terminal.locator('#shell-result').evaluate(el => el.classList.contains('on')),
          true, 'caught result card must be visible');
        await terminal.screenshot({ path: dir + '/caught-physical-fixture.png' });
        await terminal.locator('#shell-again-btn').click();
        await terminal.waitForFunction(() => {
          const s = window.__dqTest?.snapshot?.();
          return s && !s.ended && s.variables.chaser_gap >= 220 && s.t < 2;
        }, null, { timeout: 15000 });
        const restarted = await terminal.evaluate(() => window.__dqTest.snapshot());
        assert.equal(restarted.won, false);
        assert.equal(restarted.score, 0);
        await terminal.screenshot({ path: dir + '/restarted-canonical.png' });
        report.caughtFixture = true;
        report.restartVerified = true;
        report.screenshots.push('caught-physical-fixture', 'restarted-canonical');
        writeFileSync(dir + '/report.json', JSON.stringify(report, null, 2));
      } finally {
        await terminal.close();
      }
      console.log(JSON.stringify(report));
    } catch (error) {
      await capture('failed').catch(() => {});
      results.push({ browser: browserName, viewport: profile.name, passed: false,
        error: String(error), errors });
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}
const report = { browser: browserName, passed: results.length === 2 && results.every(r => r.passed), results };
writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
console.log(JSON.stringify(report));
if (!report.passed) process.exitCode = 1;
