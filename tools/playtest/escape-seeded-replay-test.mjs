#!/usr/bin/env node
// Replay determinism proof for the Escape questionnaire's lane hazards.
// Uses the real Pixi/Rapier fixed-step player in probe-only capture mode,
// NOT a mocked RNG, modified IR, or a fabricated victory.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { chromium, webkit } from 'playwright';
import { PNG } from 'pngjs';

const arg = (key, fallback) => {
  const i = process.argv.indexOf('--' + key);
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099').replace(/[/]$/, '');
const browserName = arg('browser', 'chromium');
const shots = arg('shots', '/tmp/escape-seeded-replay');
const engine = browserName === 'webkit' ? webkit : chromium;
const seed = 271828;
const step = 600; // 10 real simulated seconds, including first hazard respawn
const browser = await engine.launch({ headless: true });
mkdirSync(shots, { recursive: true });

function normalized(s) {
  const ids = new Set(['player', 'chaser', 'syringe', 'syringe02', 'star', 'star_gold']);
  const fix = n => Math.round(n * 1000) / 1000;
  return {
    step: s.simStep, score: s.score, ended: s.ended, won: s.won,
    gap: fix(s.variables.chaser_gap), lane: s.player?.laneIndex,
    entities: s.entities.filter(e => ids.has(e.def))
      .map(e => ({ id: e.def, x: fix(e.x), y: fix(e.y) }))
      .sort((a, b) => a.id.localeCompare(b.id)),
  };
}

function diffRatio(first, second) {
  const a = PNG.sync.read(readFileSync(first));
  const b = PNG.sync.read(readFileSync(second));
  assert.equal(a.width, b.width);
  assert.equal(a.height, b.height);
  let sampled = 0, different = 0;
  for (let i = 0; i < a.data.length; i += 16) {
    sampled++;
    if (Math.abs(a.data[i] - b.data[i]) +
        Math.abs(a.data[i + 1] - b.data[i + 1]) +
        Math.abs(a.data[i + 2] - b.data[i + 2]) > 42) different++;
  }
  return different / Math.max(1, sampled);
}

async function run(profile, label) {
  const page = await browser.newPage({
    viewport: { width: profile.width, height: profile.height },
    hasTouch: true, deviceScaleFactor: 1,
  });
  const errors = [];
  page.on('pageerror', error => errors.push(String(error)));
  const filename = shots + '/' + profile.name + '-' + label + '.png';
  try {
    await page.goto(base + '/player?game=escape-from-injection&presentation=play&probe=1&capture=1&seed=' + seed);
    await page.waitForFunction(() => window.__dqTest?.snapshot?.()?.capturePaused === true,
      null, { timeout: 20000 });
    const initial = await page.evaluate(() => window.__dqTest.snapshot());
    assert.equal(initial.simStep, 0, 'capture must start at authoritative simulation frame zero');
    assert.equal(initial.player?.laneIndex, 1);
    assert.equal(await page.evaluate(target => window.__dqTest.advanceToStep(target), step),
      true, 'fixed-step probe must accept target');
    await page.waitForFunction(target => {
      const s = window.__dqTest?.snapshot?.();
      return s && s.simStep === target && s.capturePaused;
    }, step, { timeout: 30000 });
    const result = normalized(await page.evaluate(() => window.__dqTest.snapshot()));
    assert.equal(result.ended, false, 'the unmodified chase must remain playable at ten seconds');
    assert.ok(result.gap >= 55 && result.gap <= 250);
    for (const e of result.entities.filter(e => e.id.startsWith('syringe'))) {
      assert.ok([97.5, 195, 292.5].some(x => Math.abs(e.x - x) < 2),
        'falling syringe must remain in an actual lane after seeded recycling');
    }
    // Let the compositor show exactly the paused fixed simulation state.
    await page.waitForTimeout(120);
    await page.screenshot({ path: filename });
    assert.deepEqual(errors, [], 'fixed-step replay had no runtime exceptions');
    return { state: result, screenshot: filename };
  } finally {
    await page.close();
  }
}

const reports = [];
try {
  for (const profile of [
    { name: 'iphone-portrait', width: 390, height: 844 },
    { name: 'ipad-portrait', width: 820, height: 1180 },
  ]) {
    const a = await run(profile, 'first');
    const b = await run(profile, 'replay');
    assert.deepEqual(b.state, a.state,
      'same seed and no inputs must reproduce full canonical physics/score/gap at same fixed tick');
    const screenshotDiff = diffRatio(a.screenshot, b.screenshot);
    assert.ok(screenshotDiff < 0.03,
      'the two fixed-step render captures must agree visually; diff=' + screenshotDiff);
    reports.push({
      passed: true, browser: browserName, viewport: profile.name,
      seed, simStep: step, state: a.state, screenshotDifference: screenshotDiff,
      screenshots: [a.screenshot, b.screenshot], mode: 'canonical-fixed-step-two-runs',
      provesFullNaturalTwentyMinutes: false,
    });
    console.log(JSON.stringify(reports.at(-1)));
  }
} finally {
  await browser.close();
}
writeFileSync(shots + '/report.json',
  JSON.stringify({ passed: reports.length === 2, browser: browserName, results: reports }, null, 2));
