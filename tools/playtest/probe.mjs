#!/usr/bin/env node
// DoodleQuest playtest worker (fused detect-and-fix flow, Phase 1).
//
// Boots every game in a real browser (Chromium or WebKit), reads the
// player's ?probe=1 verdict, then PLAYS each game with simple bot input
// (arrow keys + drags) while watching the live HUD/status/error buffer.
// Emits one JSON report on stdout in the validator's check vocabulary so
// the Go side (internal/playtest + validator.MergePlaytest) can fold the
// measurements into the step-4 ValidationReport, replacing skip entries.
//
// Honesty rules: a check the bot could not measure is reported as "skip"
// with the reason — never as "pass". A bot that fails to reach a win does
// NOT fail simulation.win_reachable (bot incompetence is not evidence);
// win_reachable only passes on an observed win and stays skip otherwise.
// The gate-failing conditions are boot errors, a blank stage, and a
// frozen stage under input (softlock signature). Timing checks are
// reported but do not gate — shared CI runners make them noise.
//
// Usage:
//   node probe.mjs --base http://localhost:8080 [--games a,b,c]
//                  [--browser chromium|webkit] [--play-seconds 14]
//                  [--win-seconds 40] [--shots <dir>]
// Exit code: 0 when no game FAILs the gate checks, 1 otherwise, 2 on
// worker-level failure (browser missing, server unreachable).

import { chromium, webkit } from 'playwright';
import { scoreGame } from './quality-score.mjs';
import { mkdirSync, writeFileSync } from 'node:fs';

function arg(name, dflt) {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : dflt;
}

const base = (arg('base', '') || '').replace(/\/$/, '');
const browserName = arg('browser', 'chromium');
const playSeconds = Number(arg('play-seconds', '14'));
const winSeconds = Number(arg('win-seconds', '40'));
const shotsDir = arg('shots', '');
if (!base) {
  console.error('probe.mjs: --base is required');
  process.exit(2);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function feedGames() {
  const res = await fetch(base + '/v1/feed');
  if (!res.ok) throw new Error('feed returned ' + res.status);
  const body = await res.json();
  const games = Array.isArray(body) ? body : (body.games || []);
  return games.map((g) => g.game_id || g.id).filter(Boolean);
}

function scoreOf(hud) {
  // HUD looks like "★ 3" / "★ 3 ♥ 2": the score is the first number group.
  const m = /(\d+)/.exec(hud || '');
  return m ? Number(m[1]) : 0;
}

async function playGame(browser, gameId) {
  const checks = [];
  const push = (checkId, status, evidence, actual) =>
    checks.push({ checkId, status, evidence, actual });
  const page = await browser.newPage({ viewport: { width: 420, height: 900 } });
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message || e)));
  const navStart = Date.now();
  let probe = null;
  try {
    await page.goto(`${base}/player?game=${encodeURIComponent(gameId)}&probe=1`,
      { waitUntil: 'load', timeout: 30000 });
    // Wait for the probe verdict to show a booted game with frames.
    const deadline = Date.now() + 25000;
    while (Date.now() < deadline) {
      probe = await page.evaluate(() => window.__dqProbe || null).catch(() => null);
      if (probe && probe.booted && probe.frames > 0) break;
      if (probe && probe.errors && probe.errors.length > 0) break;
      await sleep(500);
    }
  } catch (e) {
    push('playtest.boot_clean', 'fail', 'page load failed: ' + e.message, null);
    push('playtest.render_nonblank', 'skip', 'page never loaded', null);
    await page.close().catch(() => {});
    return { gameId, verdict: 'FAIL', checks, probe, quality: scoreGame(checks) };
  }
  const coldStartMs = Date.now() - navStart;

  // --- boot_clean -------------------------------------------------------
  const errs = [...(probe?.errors || []), ...pageErrors.map((m) => ({ message: m }))];
  if (probe && probe.booted && errs.length === 0) {
    push('playtest.boot_clean', 'pass',
      `booted, ${probe.frames} frames at first verdict, 0 errors`, { frames: probe.frames });
  } else {
    push('playtest.boot_clean', 'fail',
      'boot errors: ' + (errs.map((e) => e.message).join(' | ') || 'not booted'),
      { booted: !!(probe && probe.booted) });
  }

  // The runtime computes its stage census at frame 30 and the probe
  // verdict re-emits every 2s — the first verdict (frames ~1) has no
  // census yet. Wait for a verdict that carries one before judging
  // whether the stage is drawn; an early empty census is a sampling
  // artifact, not a blank stage (CI, 2026-10-22: both engines failed
  // render_nonblank on exactly this).
  {
    const censusDeadline = Date.now() + 9000;
    while (Date.now() < censusDeadline) {
      const fresh = await page.evaluate(() => window.__dqProbe || null).catch(() => null);
      if (fresh) probe = fresh;
      if (probe && probe.census) break;
      await sleep(400);
    }
  }

  // --- render_nonblank --------------------------------------------------
  const census = (probe && probe.census) || '';
  const entityHits = (census.match(/@\(/g) || []).length - 1; // minus worldLayer
  const anyVisible = /vis=true/.test(census);
  if (entityHits >= 1 && anyVisible) {
    push('playtest.render_nonblank', 'pass',
      `probe census shows ${entityHits} sampled entities, visible: ${census.slice(0, 160)}`,
      { sampledEntities: entityHits });
  } else {
    push('playtest.render_nonblank', 'fail',
      'probe census empty or no visible entity: ' + (census || '(none)'), null);
  }

  // --- budgets.cold_start (contract: <= 3000ms to playable) ------------
  push('budgets.cold_start', coldStartMs <= 3000 ? 'pass' : 'fail',
    `navigation to booted-with-frames in ${coldStartMs}ms (contract <= 3000ms)`,
    { coldStartMs });

  // --- hook.time_to_input (contract: input accepted within 5s) ---------
  let inputReadyMs = null;
  {
    const t0 = Date.now();
    while (Date.now() - t0 < 6000) {
      const status = await page.evaluate(
        () => document.getElementById('status')?.textContent || '').catch(() => '');
      if (/drag|tap|arrow|move|wasd/i.test(status)) { inputReadyMs = Date.now() - navStart; break; }
      await sleep(250);
    }
    if (inputReadyMs !== null) {
      push('hook.time_to_input', inputReadyMs <= 5000 ? 'pass' : 'fail',
        `input prompt visible at ${inputReadyMs}ms`, { inputReadyMs });
    } else {
      push('hook.time_to_input', 'skip', 'no input prompt observed within 6s', null);
    }
  }

  // --- bot play: first reward, softlock, win ----------------------------
  const hud0 = await page.evaluate(
    () => document.getElementById('score')?.textContent || '').catch(() => '');
  const score0 = scoreOf(hud0);
  let firstRewardMs = null;
  let stateChanged = false;
  let winSeen = false;
  let endSeen = false;
  let lastSig = '';
  const playDeadline = Date.now() + Math.min(playSeconds, winSeconds) * 1000;
  const keys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'];
  let ki = 0;
  while (Date.now() < playDeadline) {
    await page.keyboard.down(keys[ki % keys.length]).catch(() => {});
    await sleep(450);
    await page.keyboard.up(keys[ki % keys.length]).catch(() => {});
    ki++;
    if (ki % 4 === 0) {
      await page.mouse.move(210, 450).catch(() => {});
      await page.mouse.down().catch(() => {});
      await page.mouse.move(300, 380, { steps: 8 }).catch(() => {});
      await page.mouse.up().catch(() => {});
    }
    const live = await page.evaluate(() => ({
      hud: document.getElementById('score')?.textContent || '',
      status: document.getElementById('status')?.textContent || '',
      errors: (window.__dqErrors || []).length,
      census: (window.__dqProbe || {}).census || '',
      frames: (window.__dqProbe || {}).frames || 0,
      endVisible: (() => { const e = document.getElementById('end'); return !!e && getComputedStyle(e).display !== 'none'; })(),
      endTitle: document.getElementById('end-title')?.textContent || '',
    })).catch(() => null);
    if (!live) break;
    // Liveness signature: HUD, status, error count AND the stage census
    // (entity positions — a live physics world moves even when the bot
    // scores nothing; a frozen census across input is the softlock
    // signature). HUD alone false-failed Ribbon in CI: the bot caught a
    // shard before the play window, then nothing changed for 14s in a
    // perfectly alive game.
    const sig = live.hud + '|' + live.status + '|' + live.errors + '|' + live.census;
    if (lastSig && sig !== lastSig) stateChanged = true;
    lastSig = sig;
    if (firstRewardMs === null && scoreOf(live.hud) > score0) {
      firstRewardMs = Date.now() - navStart;
    }
    if (live.endVisible) {
      endSeen = true;
      if (/win|did it|complete|escaped|mended|great|hooray/i.test(live.endTitle) ||
          !/caught|got you|grabbed|game over/i.test(live.endTitle)) {
        winSeen = true;
      }
      break;
    }
  }
  const liveErrs = await page.evaluate(() => (window.__dqErrors || []).length).catch(() => 0);
  if (liveErrs > 0 || pageErrors.length > 0) {
    // Errors during play downgrade boot_clean retroactively.
    const bc = checks.find((c) => c.checkId === 'playtest.boot_clean');
    if (bc && bc.status === 'pass') {
      bc.status = 'fail';
      bc.evidence = `errors appeared during bot play (live buffer ${liveErrs}, page ${pageErrors.length})`;
    }
  }

  if (firstRewardMs !== null) {
    push('hook.first_reward', firstRewardMs <= 15000 + coldStartMs ? 'pass' : 'fail',
      `first score increase at ${firstRewardMs}ms from navigation`, { firstRewardMs });
  } else if (endSeen) {
    push('hook.first_reward', 'skip', 'game ended before any score change was observed', null);
  } else {
    push('hook.first_reward', 'skip',
      `${playSeconds}s of bot play produced no score change; reward timing unproven`, null);
  }

  if (winSeen) {
    push('simulation.win_reachable', 'pass', 'bot observed the win/end overlay', { win: true });
  } else {
    push('simulation.win_reachable', 'skip',
      'bot did not reach a win inside the play budget; reachability unproven (not a fail)', null);
  }

  if (stateChanged || endSeen) {
    push('simulation.no_softlock', 'pass',
      'HUD/status/error state changed during bot play; no frozen state observed', null);
  } else if (liveErrs > 0) {
    push('simulation.no_softlock', 'skip', 'state frozen but errors occurred; inconclusive', null);
  } else {
    push('simulation.no_softlock', 'fail',
      `no HUD/status/census movement across ${playSeconds}s of bot input and no end state: softlock signature`, null);
  }

  if (shotsDir) {
    mkdirSync(shotsDir, { recursive: true });
    await page.screenshot({ path: `${shotsDir}/${gameId}.png` }).catch(() => {});
  }
  await page.close().catch(() => {});
  // Exit-gate checks: breakage signals only. budgets.cold_start is
  // reported (and lands in merged ValidationReports) but does NOT gate:
  // on shared CI runners its timing is environment noise, and a gate
  // that fires on noise teaches everyone to ignore the gate.
  const failed = checks.some((c) => c.status === 'fail' &&
    (c.checkId === 'playtest.boot_clean' || c.checkId === 'playtest.render_nonblank' ||
     c.checkId === 'simulation.no_softlock'));
  return { gameId, verdict: failed ? 'FAIL' : 'PASS', checks, probe, quality: scoreGame(checks) };
}

async function main() {
  const gameIdsArg = (arg('games', '') || '').split(',').map((s) => s.trim()).filter(Boolean);
  const engine = browserName === 'webkit' ? webkit : chromium;
  const browser = await engine.launch();
  try {
    const gameIds = gameIdsArg.length ? gameIdsArg : await feedGames();
    const games = [];
    for (const id of gameIds) games.push(await playGame(browser, id));
    const report = { base, browser: browserName, games };
    const out = JSON.stringify(report, null, 1);
    if (shotsDir) {
      mkdirSync(shotsDir, { recursive: true });
      writeFileSync(`${shotsDir}/playtest-report.json`, out);
    }
    console.log(out);
    process.exit(games.some((g) => g.verdict === 'FAIL') ? 1 : 0);
  } finally {
    await browser.close().catch(() => {});
  }
}

main().catch((e) => {
  console.error('probe.mjs: worker failure: ' + (e && e.stack || e));
  process.exit(2);
});
