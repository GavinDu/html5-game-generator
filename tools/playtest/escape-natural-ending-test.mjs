#!/usr/bin/env node
// Verify the real, unmodified Escape story reaches its authored 'caught'
// result, including the 1200s ceiling. The fixed-step Pixi/Rapier engine
// advances simulation quickly: NO shortened timer, injected collision,
// modified Game IR, mutable state hook, or fabricated terminal event.
// An idle runner can be caught early by hazards; that DOES NOT prove
// a skilled player can naturally survive 15-20 minutes.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i < 0 ? fallback : process.argv[i + 1];
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/escape-natural-end');
const engine = browserName === 'webkit' ? webkit : chromium;
const seed = 271828;
const finalStep = 72_500; // 1208.3 simulated seconds; catch must happen by 1200s
const results = [];
mkdirSync(shots, { recursive: true });
if (!['chromium', 'webkit'].includes(browserName)) throw Error('unknown engine');

const browser = await engine.launch({ headless: true });
try {
  for (const profile of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const page = await browser.newPage({
      viewport: { width: profile.width, height: profile.height },
      hasTouch: true, deviceScaleFactor: 1,
    });
    const errors = [];
    const routeRuleLogs = [];
    page.on('pageerror', e => errors.push(String(e)));
    page.on('console', m => {
      const line = m.text();
      if (line.includes("rule 'progress-")) routeRuleLogs.push(line);
    });
    const out = shots + '/' + profile.name;
    mkdirSync(out, { recursive: true });
    try {
      const res = await page.request.get(base + '/v1/games/escape-from-injection/package');
      assert.equal(res.status(), 200);
      const pkg = await res.json();
      const rules = pkg.ir?.rules || [];
      const catchRule = rules.find(r => r.id === 'gap-caught');
      const ceiling = rules.find(r => r.id === 'inevitable-catch');
      assert.ok(catchRule?.actions?.some(a => a.type === 'end_game' && a.value === false),
        'unmodified authored caught-ending rule required');
      assert.equal(ceiling?.when?.event, 'timer_elapsed');
      assert.equal(ceiling?.when?.value, 1200,
        'long-run test must not change the authored 20-minute ceiling');
      assert.equal(pkg.ir.experience.chaserGap.start, 220);
      assert.equal(pkg.ir.experience.chaserGap.caughtAt, 55);

      await page.goto(base + '/player?game=escape-from-injection&presentation=play' +
        '&probe=1&capture=1&seed=' + seed, { waitUntil: 'load', timeout: 30000 });
      await page.waitForFunction(() => {
        const state = window.__dqTest?.snapshot?.();
        return state?.capturePaused && state.simStep === 0;
      }, null, { timeout: 30000 });
      const initial = await page.evaluate(() => window.__dqTest.snapshot());
      assert.equal(initial.ended, false);
      assert.equal(initial.player?.laneIndex, 1);
      assert.equal(await page.evaluate(() => window.__dqTest.advanceToStep(600)), true);
      await page.waitForFunction(() => {
        const s = window.__dqTest?.snapshot?.();
        return s?.ended || (s?.capturePaused && s.simStep === 600);
      }, null, { timeout: 40000 });
      const tenSeconds = await page.evaluate(() => window.__dqTest.snapshot());
      assert.ok(tenSeconds.t >= 9.9, 'the real game must advance ten seconds');
      assert.ok(tenSeconds.score >= 0 && tenSeconds.won === false);
      await page.screenshot({ path: out + '/ten-seconds-real-gameplay.png' });

      // The canonical chase must remain alive long enough to show both
      // 40-second route transitions. This uses unmodified IR and real
      // Pixi/Rapier fixed steps, not shortened timer fixtures.
      assert.equal(tenSeconds.ended, false,
        'unmodified runner must remain alive through the first ten seconds');
      const advanceAndInspect = async (target) => {
        assert.equal(await page.evaluate(step => window.__dqTest.advanceToStep(step), target),
          true, 'fixed-step capture must accept the route milestone');
        await page.waitForFunction(step => {
          const snap = window.__dqTest?.snapshot?.();
          return snap?.ended || (snap?.capturePaused && snap.simStep === step);
        }, target, { timeout: 45000 });
        const snap = await page.evaluate(() => window.__dqTest.snapshot());
        assert.equal(snap.ended, false, 'route milestones must occur before a natural caught ending');
        return snap;
      };
      const firstRoute = await advanceAndInspect(2460); // 41 simulated seconds
      const firstMid = firstRoute.entities.find(e => e.def === 'hall-mid');
      const firstFar = firstRoute.entities.find(e => e.def === 'hall-far');
      assert.ok(firstMid?.alpha > 0.9 && firstFar?.alpha < 0.1,
        'at 41s only the first authored hallway route should be active');
      assert.equal(routeRuleLogs.filter(m => m.includes("rule 'progress-40'")).length, 1,
        'shared 40s timer must fire the first route once, not once per subscribing rule');
      assert.equal(routeRuleLogs.filter(m => m.includes("rule 'progress-80'")).length, 0);
      await page.screenshot({ path: out + '/route-after-40s.png' });

      const secondRoute = await advanceAndInspect(4860); // 81 simulated seconds
      const secondMid = secondRoute.entities.find(e => e.def === 'hall-mid');
      const secondFar = secondRoute.entities.find(e => e.def === 'hall-far');
      assert.ok(secondMid?.alpha < 0.1 && secondFar?.alpha > 0.9,
        'at 81s the second route must replace the first rather than overlay it');
      assert.equal(routeRuleLogs.filter(m => m.includes("rule 'progress-40'")).length, 1);
      assert.equal(routeRuleLogs.filter(m => m.includes("rule 'progress-80'")).length, 1,
        'shared 40s timer must fire the second route exactly once at 80s');
      await page.screenshot({ path: out + '/route-after-80s.png' });

      assert.equal(await page.evaluate(step => window.__dqTest.advanceToStep(step), finalStep),
        true, 'fixed-step capture must accept the real full-duration target');
      await page.waitForFunction(target => {
        const s = window.__dqTest?.snapshot?.();
        return s?.ended || (s?.capturePaused && s.simStep === target);
      }, finalStep, { timeout: 120000 });
      const ending = await page.evaluate(() => window.__dqTest.snapshot());
      assert.equal(ending.ended, true,
        'canonical, unmodified Escape must reach its caught terminal by 20 minutes');
      assert.equal(ending.won, false, 'Escape story must never fabricate a victory');
      assert.ok(ending.t <= 1202, 'caught ending must not extend past 20-minute ceiling');
      assert.ok(ending.variables.chaser_gap <= 55 + 0.5,
        'only depleted authoritative chaser lead can end a canonical run');
      const resultVisible = await page.locator('#shell-result')
        .evaluate(el => el.classList.contains('on'));
      assert.equal(resultVisible, true, 'real caught ending must show a restartable result card');
      assert.deepEqual(errors, [], 'no uncaught browser errors in the long-run simulation');
      await page.screenshot({ path: out + '/natural-caught-result.png' });
      results.push({
        viewport: profile.name, browser: browserName, passed: true, seed,
        firstTenSeconds: tenSeconds.t, endingTime: ending.t,
        endingStep: ending.simStep, endingScore: ending.score,
        endingGap: ending.variables.chaser_gap, won: ending.won,
        naturallyCaught: true, canonicalTimerUnchanged: true,
        route40FiredExactlyOnce: true, route80FiredExactlyOnce: true,
        routeScreenshots: ['route-after-40s.png', 'route-after-80s.png'],
        fullDurationReached: ending.t >= 1199,
        provesSkilledFifteenToTwentyMinuteSurvival: false,
        screenshots: ['ten-seconds-real-gameplay.png', 'route-after-40s.png',
          'route-after-80s.png', 'natural-caught-result.png'],
      });
    } catch (e) {
      await page.screenshot({ path: out + '/failed.png' }).catch(() => {});
      results.push({ viewport: profile.name, browser: browserName,
        passed: false, error: String(e), errors });
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}
const report = { browser: browserName, passed: results.length === 2 &&
  results.every(r => r.passed), results };
writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
console.log(JSON.stringify(report));
if (!report.passed) process.exitCode = 1;
