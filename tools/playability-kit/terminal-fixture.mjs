import fs from 'node:fs';
import path from 'node:path';

// A fixture may force collision geometry closer together; the production
// package, backend state, and canonical IR are never mutated by this helper.
export function makeCaughtFixture(pkg) {
  const data = JSON.parse(JSON.stringify(pkg));
  const ir = data.ir;
  if (!ir?.experience?.chaserGap || ir.experience.controls?.scheme !== 'lane_tap') return null;
  const hit = ir.rules?.find(rule => rule.when?.event === 'collision' &&
    rule.actions?.some(a => a.type === 'add_variable' &&
      a.variable === 'chaser_gap' && Number(a.value) < 0));
  const player = ir.scenes?.flatMap(s => s.entities || []).find(e => e.id === 'player');
  const hazard = ir.scenes?.flatMap(s => s.entities || []).find(e => e.id === hit?.when?.target);
  if (!hit || !player || !hazard || !hazard.physics) return null;
  const gap = ir.experience.chaserGap;
  const start = Math.min(gap.start, gap.caughtAt + 15);
  if (!(start > gap.caughtAt && start < gap.max &&
      start + Number(hit.actions.find(a => a.variable === 'chaser_gap').value) <= gap.caughtAt)) return null;
  gap.start = start;
  hazard.position = [...player.position];
  delete hazard.behavior;
  hazard.physics.body = 'dynamic';
  hazard.physics.sensor = true;
  hazard.physics.gravityScale = 0;
  return data;
}

export async function verifyCaughtFixture(ctx, { base, id, outDir }) {
  const page = await ctx.newPage();
  let patches = 0;
  const errors = [];
  page.on('pageerror', err => errors.push(String(err)));
  const screenshots = [];
  try {
    await page.route('**/v1/games/*/package', async route => {
      const response = await route.fetch();
      const pkg = await response.json();
      // Only the first request is modified. Play Again MUST restart the
      // real canonical package, not the accelerated test collision.
      const fixture = patches++ === 0 ? makeCaughtFixture(pkg) : null;
      if (patches === 1 && !fixture) throw new Error('no physical-collision fixture for ' + id);
      await route.fulfill({ response, json: fixture || pkg });
    });
    await page.goto(base + '/player?game=' + encodeURIComponent(id) + '&presentation=play&probe=1');
    await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.ended,
      null, { timeout: 16000 });
    const result = await page.evaluate(() => window.__dqTest.snapshot());
    const visible = await page.locator('#shell-result').evaluate(e => e.classList.contains('on'));
    const title = await page.locator('#shell-result-title').textContent();
    if (!visible || result.won || result.variables.chaser_gap > 55 || !title?.trim()) {
      throw new Error('caught result not proved: visible=' + visible + ', won=' + result.won);
    }
    const caughtShot = path.join(outDir, id + '.fixture-caught.png');
    await page.screenshot({ path: caughtShot });
    screenshots.push(caughtShot);
    await page.locator('#shell-again-btn').click();
    await page.waitForFunction(() => {
      const s = window.__dqTest?.snapshot?.();
      return !!s && !s.ended && s.score === 0 && s.variables.chaser_gap >= 220 && s.t < 2;
    }, null, { timeout: 16000 });
    const restartShot = path.join(outDir, id + '.fixture-restart.png');
    await page.screenshot({ path: restartShot });
    screenshots.push(restartShot);
    if (errors.length) throw new Error('browser errors in controlled fixture: ' + errors.slice(0, 2).join('; '));
    return { passed: true, screenshots, mode: 'controlled-physics-fixture', canonicalLongRunObserved: false };
  } catch (err) {
    const failedShot = path.join(outDir, id + '.fixture-failed.png');
    await page.screenshot({ path: failedShot }).catch(() => {});
    return { passed: false, reason: String(err), screenshots: [failedShot], canonicalLongRunObserved: false };
  } finally {
    await page.close();
  }
}
