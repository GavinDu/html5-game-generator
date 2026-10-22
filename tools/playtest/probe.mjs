// probe.mjs — headless playtest probe (auto detect-and-fix flow, Phase 1 seed).
//
// Loads every game in the feed at /player?game=<id>&probe=1 in headless
// Chromium, reads the page's own probe verdict (#probe-result), watches
// for page errors, and takes two screenshots to detect motion. Prints a
// JSON report; exit code is non-zero if any game FAILs.
//
//   node tools/playtest/probe.mjs [baseUrl] [gameId ...]
//   baseUrl defaults to https://html-5-game-generator.replit.app
//
// Requires: npm i playwright (and a Chromium build — CI provides one).
// The verdict semantics come from the runtime's probe contract:
//   booted=true, errors=[], frames>0  -> the game actually ran.
// A screenshot that is only the scene background still passes the DOM
// checks, so the motion/blank heuristics here are deliberately WARN-level
// until the pixel-variance check lands in the full worker.

import { chromium } from 'playwright';

const base = (process.argv[2] || 'https://html-5-game-generator.replit.app').replace(/\/$/, '');
let gameIds = process.argv.slice(3);

if (gameIds.length === 0) {
  const feed = await (await fetch(`${base}/v1/feed`)).json();
  gameIds = (feed.games || []).map((g) => g.game_id);
}

const browser = await chromium.launch();
const report = [];
let anyFail = false;

for (const id of gameIds) {
  const page = await browser.newPage({ viewport: { width: 430, height: 932 } });
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e.message || e)));
  let verdict = null;
  try {
    await page.goto(`${base}/player?game=${encodeURIComponent(id)}&probe=1`, { waitUntil: 'load', timeout: 30000 });
    await page.waitForSelector('#probe-result', { timeout: 40000 });
    verdict = JSON.parse(await page.textContent('#probe-result'));
  } catch (e) {
    pageErrors.push('probe wait failed: ' + String(e.message || e));
  }
  const shot1 = await page.screenshot().catch(() => null);
  await page.waitForTimeout(3000);
  const shot2 = await page.screenshot().catch(() => null);
  const motion = !!(shot1 && shot2 && !shot1.equals(shot2));
  await page.close();

  const problems = [];
  if (pageErrors.length) problems.push('page errors: ' + pageErrors.join(' | '));
  if (!verdict) problems.push('no probe verdict produced');
  if (verdict && !verdict.booted) problems.push('game did not boot');
  if (verdict && verdict.errors?.length) problems.push('runtime errors: ' + verdict.errors.map((e) => e.kind + ': ' + e.message).join(' | '));
  if (verdict && verdict.frames < 30) problems.push(`too few frames rendered (${verdict.frames})`);
  const status = problems.length ? 'FAIL' : (!motion || (verdict?.warnCount ?? 0) > 1 ? 'WARN' : 'PASS');
  if (status === 'FAIL') anyFail = true;
  report.push({ game: id, status, motion, frames: verdict?.frames ?? 0, hud: verdict?.hud ?? '', problems });
}

await browser.close();
console.log(JSON.stringify({ base, report }, null, 1));
process.exit(anyFail ? 1 : 0);
