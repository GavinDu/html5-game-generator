#!/usr/bin/env node
// Source-drawing fidelity: separate 12-heart objective from 10/30 score.
// Real touch input, recycling, aurora bands, all scarf variants and restart
// are verified separately at iPhone/iPad viewports with screenshot artifacts.
import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/penguin-quality');
const engine = browserName === 'webkit' ? webkit : chromium;
mkdirSync(shots, { recursive: true });
const browser = await engine.launch({ headless: true });
const reports = [];
try {
  for (const profile of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const dir = shots + '/' + profile.name;
    mkdirSync(dir, { recursive: true });
    const page = await browser.newPage({
      viewport: { width: profile.width, height: profile.height },
      hasTouch: true,
      deviceScaleFactor: 1,
    });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    try {
      await page.goto(base + '/player?game=penguin-heartlight-rescue&presentation=play&probe=1');
      await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.t > .1, null, { timeout: 20000 });
      const snap = () => page.evaluate(() => window.__dqTest.snapshot());
      // Canonical Game IR keeps the authored collision size and lane but
      // scales only the rendered sprite, so visual polish cannot create a
      // much larger invisible pickup hitbox.
      const pkgResponse = await page.request.get(base + '/v1/games/penguin-heartlight-rescue/package');
      assert.equal(pkgResponse.status(), 200);
      const pkg = await pkgResponse.json();
      const playerDef = pkg.ir.scenes.flatMap(scene => scene.entities).find(e => e.id === 'penguin');
      assert.ok(playerDef, 'canonical penguin must exist');
      assert.deepEqual(playerDef.size, [112, 134], 'catcher collision footprint must not be inflated');
      assert.deepEqual(playerDef.render?.scale, [1.5, 1.5],
        'hero must be visibly readable at phone/tablet scale without changing physics');
      // Screenshot-backed readability: hearts pass through the title area while
      // falling. Protect authored text without moving pickups or changing
      // collision physics; the panel is rendered above hearts, below labels.
      const allEntities = pkg.ir.scenes.flatMap(scene => scene.entities);
      const panel = allEntities.find(entity => entity.id === 'instructions-panel');
      const title = allEntities.find(entity => entity.id === 'title');
      const hint = allEntities.find(entity => entity.id === 'controls-hint');
      const pickups = allEntities.filter(entity => entity.kind === 'pickup');
      assert.ok(panel && title && hint && pickups.length >= 12,
        'a visible instruction panel, title, and tap hint must coexist with pickups');
      assert.equal(panel.kind, 'decoration', 'the instruction panel has no collision behavior');
      assert.deepEqual(panel.components, [], 'the instruction panel must remain visual-only');
      assert.equal(panel.render?.shape?.type, 'polygon');
      assert.ok(panel.render.alpha >= .9 && panel.zIndex > Math.max(...pickups.map(entity => entity.zIndex || 0)),
        'the panel must visually separate moving hearts from the title');
      assert.ok(title.zIndex > panel.zIndex && hint.zIndex > panel.zIndex,
        'title and hint must be legible above the panel in screenshots');
      const manifest = JSON.parse(readFileSync(new URL('../../inventory/p3ngui/manifest.json', import.meta.url), 'utf8'));
      assert.equal(manifest.version, 5,
        'a changed visual package must not reuse stale poster provenance');
      const first = await snap();
      assert.ok(first.entities.find(e => e.id === 'instructions-panel')?.alpha >= .9 &&
        first.entities.find(e => e.id === 'title')?.alpha >= .9,
        'the original story title must remain visible at the start of the game');
      const introRule = pkg.ir.rules.find(rule =>
        rule.id === 'intro-overlay-clear-after-seven-seconds');
      assert.equal(introRule?.when?.event, 'timer_elapsed');
      assert.equal(introRule?.when?.value, 7);
      assert.ok(introRule.actions.some(a => a.type === 'set_alpha' &&
        a.target === 'instructions-panel' && a.value === 0));
      assert.ok(introRule.actions.some(a => a.type === 'set_alpha' &&
        a.target === 'title' && a.value === 0));
      const variants = new Set([first.entities.find(e => e.id === 'penguin')?.assetId]);
      assert.equal(first.objectiveProgress, 0, 'the 12-heart counter must start at zero');
      const rect = await page.locator('#stage canvas').boundingBox();
      assert.ok(rect && rect.width > 100);
      const tap = async x => page.touchscreen.tap(
        rect.x + x / 390 * rect.width, rect.y + rect.height * .7);
      await page.screenshot({ path: dir + '/before.png' });
      await tap(58);
      await page.waitForFunction(() => window.__dqTest.snapshot().player.x < 100);
      assert.ok(Math.abs((await snap()).player.y - first.player.y) < 2,
        'a short tap must preserve the catch lane');
      await page.waitForFunction(() => window.__dqTest.snapshot().entities
        .some(e => e.id === 'heart03' && e.y > 810), null, { timeout: 15000 });
      await page.screenshot({ path: dir + '/missed-heart.png' });
      await page.waitForFunction(() => window.__dqTest.snapshot().entities
        .some(e => e.id === 'heart03' && e.y < 100), null, { timeout: 15000 });
      await page.screenshot({ path: dir + '/recycled-heart.png' });
      // Do not idle the bot while observing the seven-second intro:
      // missed catches can skip the source variant milestones and alter
      // the report. Capture from the normal actively guided gameplay loop.
      let capturedIntroCleared = false;
      // The intro may be captured in the winning session or in a separate
      // real browser session if the game completes before its seven-second timer.
      // Record which one produced the screenshot, rather than implying that
      // a supplemental screenshot belongs to the guided win trace.
      let introEvidence = null;
      const observedHeartCounts = [];
      let capturedMidway = false;
      let capturedHolidayScarf = false;
      let capturedClassicScarf = false;
      const deadline = Date.now() + 70000;
      while (Date.now() < deadline) {
        const state = await snap();
        // The story board must clear during *actual ongoing guided play*,
        // without a synthetic pause or modification to heart physics.
        if (!capturedIntroCleared && state.t >= 7.2 && !state.ended) {
          assert.ok(state.entities.find(e => e.id === 'instructions-panel')?.alpha <= .01,
            'large story board must stop hiding falling hearts after seven seconds');
          assert.ok(state.entities.find(e => e.id === 'title')?.alpha <= .01,
            'large title text must clear with its background');
          assert.ok(state.entities.find(e => e.id === 'controls-hint')?.alpha >= .9,
            'compact tap guidance should persist over the unobstructed playfield');
          await page.screenshot({ path: dir + '/intro-cleared.png' });
          introEvidence = { session: 'guided-play', runtimeSeconds: state.t,
            objectiveProgress: state.objectiveProgress };
          capturedIntroCleared = true;
        }
        variants.add(state.entities.find(e => e.id === 'penguin')?.assetId);
        for (let i = 0; i < 6; i++) {
          if (state.objectiveProgress >= (i + 1) * 2) {
            assert.ok(state.entities.find(e => e.id === 'band' + i).alpha > .9,
              'each pair of hearts must visibly brighten its own aurora band');
          }
        }
        assert.ok(state.score >= state.objectiveProgress * 10,
          'score is distinct from the heart counter');
        assert.equal((state.score - state.objectiveProgress * 10) % 20, 0,
          'every bonus reward must be a full 30-point golden heart');
        if (state.objectiveProgress < 12) {
          assert.equal(state.ended, false, 'bonus points cannot end the game early');
        }
        if (observedHeartCounts.at(-1) !== state.objectiveProgress) {
          observedHeartCounts.push(state.objectiveProgress);
        }
        const penguinAsset = state.entities.find(e => e.id === 'penguin')?.assetId;
        const expectedAsset = state.objectiveProgress >= 8 ? 'penguin_classic'
          : state.objectiveProgress >= 4 ? 'penguin_holiday' : 'penguin_rainbow';
        assert.equal(penguinAsset, expectedAsset,
          'the current scarf must match the questionnaire milestones at 4 and 8 hearts');
        // Keep real rendered screenshots as reviewer evidence for all three
        // kid-drawn scarf variants, not just assertions about asset IDs.
        if (!capturedHolidayScarf && state.objectiveProgress >= 4 &&
            state.objectiveProgress < 8 && !state.ended) {
          await page.screenshot({ path: dir + '/holiday-scarf.png' });
          capturedHolidayScarf = true;
        }
        if (!capturedClassicScarf && state.objectiveProgress >= 8 && !state.ended) {
          await page.screenshot({ path: dir + '/classic-scarf.png' });
          capturedClassicScarf = true;
        }
        if (!capturedMidway && state.objectiveProgress >= 6 && !state.ended) {
          await page.screenshot({ path: dir + '/restoring-aurora.png' });
          capturedMidway = true;
        }
        if (state.ended) break;
        const picks = state.entities
          .filter(e => e.kind === 'pickup' && e.visible && e.y < state.player.y + 20)
          .sort((a, b) => b.y - a.y);
        if (picks[0]) await tap(picks[0].x);
        await page.waitForTimeout(90);
      }
      const end = await snap();
      assert.equal(end.won, true, 'twelve catches must restore the whole aurora');
      assert.equal(end.objectiveProgress, 12);
      assert.ok(end.score >= 120 && (end.score - 120) % 20 === 0,
        'the ending awards 10/30 scoring independent of catch count');
      assert.deepEqual([...variants].sort(),
        ['penguin_classic', 'penguin_holiday', 'penguin_rainbow']);
      assert.ok(capturedHolidayScarf && capturedClassicScarf,
        'both scarf milestone screenshots must be captured during real gameplay');
      // Some fast, valid runs finish twelve hearts before the seven-second
      // story timer. Preserve the guided playtest and its win assertions, but
      // verify the intro in an independent real browser session instead of
      // failing solely because the first session ended too quickly.
      // The probe advances the actual runtime clock, never a mocked snapshot.
      if (!capturedIntroCleared) {
        const introPage = await browser.newPage({
          viewport: { width: profile.width, height: profile.height },
          hasTouch: true,
          deviceScaleFactor: 1,
        });
        try {
          await introPage.goto(base + '/player?game=penguin-heartlight-rescue&presentation=play&probe=1');
          // A hands-off supplemental capture may finish early when falling
          // hearts land on the starting catcher. Steer with a real touchscreen
          // tap into the edge lane so the seven-second overlay can clear in
          // an independently running session, without mocking state or time.
          await introPage.waitForFunction(() =>
            window.__dqTest?.snapshot?.()?.t > .1, null, { timeout: 20000 });
          const introRect = await introPage.locator('#stage canvas').boundingBox();
          assert.ok(introRect && introRect.width > 100,
            'supplemental intro capture requires a real rendered stage');
          await introPage.touchscreen.tap(
            introRect.x + introRect.width * .03,
            introRect.y + introRect.height * .7
          );
          const introMoved = await introPage.evaluate(() => window.__dqTest.snapshot());
          assert.ok(introMoved.player.x < 110,
            'real touch must move the catcher away from the automatic center pickups');
          try {
            await introPage.waitForFunction(() => {
              const state = window.__dqTest?.snapshot?.();
              return state && (state.t >= 7.2 || state.ended);
            }, null, { timeout: 20000 });
          } catch (error) {
            // Keep a real screenshot and runtime state when a retry still
            // fails. Never accept a missing overlay-clear screenshot.
            await introPage.screenshot({
              path: dir + '/intro-capture-failed.png'
            }).catch(() => {});
            const state = await introPage.evaluate(() =>
              window.__dqTest?.snapshot?.()).catch(() => null);
            throw new Error('real supplemental intro capture timed out: ' +
              JSON.stringify({ runtime: state?.t, ended: state?.ended,
                progress: state?.objectiveProgress, cause: String(error) }));
          }
          const activeIntro = await introPage.evaluate(() => window.__dqTest.snapshot());
          assert.equal(activeIntro.ended, false,
            'intro evidence must come from a running game, not its end screen');
          assert.ok(activeIntro.entities.find(e => e.id === 'instructions-panel')?.alpha <= .01,
            'story panel must clear after seven seconds in a real running game');
          assert.ok(activeIntro.entities.find(e => e.id === 'title')?.alpha <= .01,
            'title must clear together with the seven-second story panel');
          assert.ok(activeIntro.entities.find(e => e.id === 'controls-hint')?.alpha >= .9,
            'compact tap guidance must remain after the story board clears');
          await introPage.screenshot({ path: dir + '/intro-cleared.png' });
          introEvidence = { session: 'independent-real-browser', runtimeSeconds: activeIntro.t,
            objectiveProgress: activeIntro.objectiveProgress };
          capturedIntroCleared = true;
        } finally {
          await introPage.close();
        }
      }
      assert.equal(capturedIntroCleared, true,
        'screen capture must prove the story overlay cleared during active play');
      assert.ok(introEvidence && introEvidence.runtimeSeconds >= 7.2 &&
        ['guided-play', 'independent-real-browser'].includes(introEvidence.session) &&
        Number.isFinite(introEvidence.objectiveProgress),
        'the intro screenshot must carry real-session runtime provenance');
      await page.screenshot({ path: dir + '/win.png' });
      await page.evaluate(() => { window.__penguinReloadMarker = 'kept'; });
      await page.locator('#shell-again-btn').click();
      await page.waitForFunction(() => {
        const state = window.__dqTest?.snapshot?.();
        return state && !state.ended && state.objectiveProgress === 0 && state.score === 0;
      }, null, { timeout: 15000 });
      assert.equal(await page.evaluate(() => window.__penguinReloadMarker), 'kept');
      const restarted = await snap();
      assert.ok(restarted.entities.find(e => e.id === 'instructions-panel')?.alpha >= .9 &&
        restarted.entities.find(e => e.id === 'title')?.alpha >= .9,
        'a fresh round must restore the source-story introduction');
      await page.screenshot({ path: dir + '/restarted.png' });
      assert.deepEqual(errors, [], 'runtime must not emit uncaught page errors');
      const report = {
        passed: true, browser: browserName, viewport: profile.name,
        missedHeartRecycled: true, shortTapMovesHorizontally: true,
        observedHeartCounts, variants: [...variants], won: end.won,
        hearts: end.objectiveProgress, score: end.score,
        goldenHeartCatches: (end.score - 120) / 20,
        brightBands: end.entities.filter(e => e.id.startsWith('band'))
          .map(e => ({ id: e.id, alpha: e.alpha })),
        restartInPlace: true, introClearedAfterSevenSeconds: true,
        introEvidence,
        introRestoredOnRestart: true, compactHintPersists: true,
        scarfMilestoneScreenshots: {
          rainbow: 'before.png', holiday: 'holiday-scarf.png', classic: 'classic-scarf.png'
        },
        screenshots: ['before', 'missed-heart', 'recycled-heart', 'intro-cleared', 'holiday-scarf',
          'restoring-aurora', 'classic-scarf', 'win', 'restarted'],
      };
      reports.push(report);
      writeFileSync(dir + '/report.json', JSON.stringify(report, null, 2));
      console.log(JSON.stringify(report));
    } catch (error) {
      await page.screenshot({ path: dir + '/failed.png' }).catch(() => {});
      reports.push({ passed: false, browser: browserName, viewport: profile.name,
        error: String(error), pageErrors: errors });
    } finally {
      await page.close();
    }
  }
} finally {
  await browser.close();
}
const report = { browser: browserName, passed: reports.length === 2 &&
  reports.every(r => r.passed), results: reports };
writeFileSync(shots + '/report.json', JSON.stringify(report, null, 2));
console.log(JSON.stringify(report));
if (!report.passed) process.exitCode = 1;
