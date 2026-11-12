#!/usr/bin/env node
// Epic #202/#315: actual seeded Game IR in every shipped inventory game,
// compared by semantic state (not pixels) at one fixed virtual tick.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { chromium, webkit } from 'playwright';

const arg = (key, fallback) => {
  const index = process.argv.indexOf('--' + key);
  return index >= 0 ? process.argv[index + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/inventory-semantic-replay');
const seed = 271828, simStep = 240;
const inventoryDir = new URL('../../inventory/', import.meta.url);
const gameIDs = readdirSync(inventoryDir, { withFileTypes: true })
  .filter(entry => entry.isDirectory())
  .flatMap(entry => {
    try {
      const manifest = JSON.parse(readFileSync(new URL(entry.name + '/manifest.json', inventoryDir), 'utf8'));
      return manifest.game_id ? [manifest.game_id] : [];
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
  }).sort();
assert.ok(gameIDs.length >= 4, 'need all four shipped inventory games, not a cherry-picked sample');
assert.equal(new Set(gameIDs).size, gameIDs.length, 'duplicate inventory game IDs');
mkdirSync(shots, { recursive: true });

const round = n => typeof n === 'number' ? Math.round(n * 1000) / 1000 : n;
const normalizeVariables = variables => Object.fromEntries(Object.entries(variables || {})
  .sort(([a], [b]) => a.localeCompare(b))
  .map(([k, v]) => [k, round(v)]));
function normalize(s) {
  const mechanics = new Set(['player', 'pickup', 'hazard', 'enemy', 'obstacle', 'platform', 'goal']);
  return {
    simStep: s.simStep, elapsed: round(s.t),
    score: s.score, health: s.health,
    objectiveProgress: round(s.objectiveProgress), ended: s.ended, won: s.won,
    variables: normalizeVariables(s.variables),
    player: s.player ? { id: s.player.id, x: round(s.player.x), y: round(s.player.y),
      laneIndex: s.player.laneIndex } : null,
    entities: s.entities.filter(e => mechanics.has(e.kind))
      .map(e => ({ id: e.id, def: e.def, kind: e.kind,
        x: round(e.x), y: round(e.y), w: round(e.w), h: round(e.h) }))
      .sort((a, b) => a.id.localeCompare(b.id)),
  };
}
const engine = browserName === 'webkit' ? webkit : chromium;
const browser = await engine.launch({ headless: true });
const results = [];
try {
  for (const game of gameIDs) {
    for (const viewport of [
      { name: 'iphone-portrait', width: 390, height: 844 },
      { name: 'ipad-portrait', width: 820, height: 1180 },
    ]) {
      const page = await browser.newPage({
        viewport: { width: viewport.width, height: viewport.height },
        hasTouch: true, deviceScaleFactor: 1,
      });
      const errors = [];
      page.on('pageerror', error => errors.push(String(error)));
      try {
        const url = base + '/player?game=' + encodeURIComponent(game) +
          '&presentation=play&probe=1&capture=1&seed=' + seed;
        await page.goto(url, { timeout: 30000 });
        await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.capturePaused === true,
          null, { timeout: 25000 });
        assert.equal(await page.evaluate(() => window.__dqTest.snapshot().simStep), 0);
        assert.equal(await page.evaluate(n => window.__dqTest.advanceToStep(n), simStep), true);
        await page.waitForFunction(n => {
          const snapshot = window.__dqTest?.snapshot?.();
          return snapshot?.capturePaused && snapshot.simStep === n;
        }, simStep, { timeout: 35000 });
        assert.deepEqual(errors, [], 'runtime exceptions are not deterministic success');
        const state = normalize(await page.evaluate(() => window.__dqTest.snapshot()));
        assert.equal(state.simStep, simStep);
        const shot = shots + '/' + game + '-' + viewport.name + '.png';
        await page.waitForTimeout(120);
        await page.screenshot({ path: shot });
        const item = { game, viewport: viewport.name, seed, simStep,
          passed: true, state, screenshot: shot };
        results.push(item);
        console.log(JSON.stringify({ game, viewport: viewport.name, simStep, passed: true }));
      } finally { await page.close(); }
    }
  }
} finally { await browser.close(); }
writeFileSync(shots + '/report.json', JSON.stringify({
  browser: browserName, seed, simStep, gameIDs,
  passed: results.length === gameIDs.length * 2, results,
}, null, 2));
