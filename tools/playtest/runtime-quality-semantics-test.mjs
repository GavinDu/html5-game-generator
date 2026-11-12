#!/usr/bin/env node
// Hephaestus #202 / generator #315: quality must not change semantics.
// Real shipped Game IR, Pixi and Rapier; the read-only fixed-step probe
// advances from tick zero. This test does not mutate or fake Game IR.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';

const arg = (key, fallback) => {
  const i = process.argv.indexOf('--' + key);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/runtime-quality-semantics');
const engine = browserName === 'webkit' ? webkit : chromium;
const seed = 271828, step = 600;
mkdirSync(shots, { recursive: true });
const browser = await engine.launch({ headless: true });
const normalize = s => {
  const ids = new Set(['player', 'chaser', 'syringe', 'syringe02', 'star', 'star_gold']);
  const round = x => Math.round(x * 1000) / 1000;
  return {
    simStep: s.simStep, score: s.score, health: s.health,
    ended: s.ended, won: s.won, lane: s.player?.laneIndex,
    chaserGap: round(s.variables.chaser_gap),
    entities: s.entities.filter(e => ids.has(e.def))
      .map(e => ({ def: e.def, x: round(e.x), y: round(e.y) }))
      .sort((a, b) => a.def.localeCompare(b.def)),
  };
};
const evidence = [];
try {
  for (const viewport of [
    { name: 'iphone', width: 390, height: 844 },
    { name: 'ipad', width: 820, height: 1180 },
  ]) {
    const snapshots = new Map();
    for (const quality of ['low', 'high']) {
      const page = await browser.newPage({ viewport: { width: viewport.width, height: viewport.height }, hasTouch: true });
      const pageErrors = [];
      page.on('pageerror', err => pageErrors.push(String(err)));
      try {
        await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1&capture=1&seed='
          + seed + '&quality=' + quality, { timeout: 30000 });
        await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.capturePaused === true,
          null, { timeout: 25000 });
        const initial = await page.evaluate(() => window.__dqTest.snapshot());
        assert.equal(initial.simStep, 0);
        assert.equal(await page.evaluate(target => window.__dqTest.advanceToStep(target), step), true);
        await page.waitForFunction(target => {
          const s = window.__dqTest?.snapshot?.();
          return s && s.capturePaused && s.simStep === target;
        }, step, { timeout: 35000 });
        const state = normalize(await page.evaluate(() => window.__dqTest.snapshot()));
        assert.deepEqual(pageErrors, [], 'browser runtime errors are unacceptable');
        assert.equal(state.ended, false);
        const screenshot = shots + '/' + viewport.name + '-' + quality + '.png';
        await page.waitForTimeout(120);
        await page.screenshot({ path: screenshot });
        snapshots.set(quality, state);
        evidence.push({ viewport: viewport.name, quality, seed, step, state, screenshot });
      } finally { await page.close(); }
    }
    assert.deepEqual(snapshots.get('high'), snapshots.get('low'),
      'render quality changed deterministic gameplay semantics on ' + viewport.name);
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, seed, step, passed: evidence.length === 4, evidence,
  note: 'fixed-step source game; simulator/browser evidence, not hardware memory or 30/120Hz device proof',
}, null, 2));
console.log(JSON.stringify({ browser: browserName, passed: true, cases: evidence.length, seed, step }));
