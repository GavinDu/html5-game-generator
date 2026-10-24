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
import { mkdirSync, writeFileSync } from 'node:fs';
import { selectFirstRewardMeasurement } from './reward-timing.mjs';

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

async function readProbe(pg) {
  return pg.evaluate(() => window.__dqProbe || null).catch(() => null);
}

// For catcher games, steer from the live player position toward the nearest
// falling pickup while keeping the pointer on the player's horizontal lane.
// Fixed viewport coordinates can move the player away from the catcher lane
// and make first-reward timing measure a bad bot, not the game.
async function catcherPointerPoints(pg, fallbackRatio = 0.5) {
  return pg.evaluate((ratio) => {
    const snap = window.__dqTest?.snapshot?.();
    const canvas = document.querySelector('#stage canvas');
    if (!snap?.player || !canvas || !snap.world?.w || !snap.world?.h) return null;
    const player = snap.player;
    const pickup = snap.entities
      .filter((e) => e.kind === 'pickup' && e.visible && e.y <= player.y + 20)
      .sort((a, b) => Math.abs(a.x - player.x) - Math.abs(b.x - player.x))[0];
    const targetX = pickup ? pickup.x : snap.world.w * ratio;
    const rect = canvas.getBoundingClientRect();
    const point = (x, y) => ({
      x: rect.left + (x / snap.world.w) * rect.width,
      y: rect.top + (y / snap.world.h) * rect.height,
    });
    return { from: point(player.x, player.y), to: point(targetX, player.y) };
  }, fallbackRatio).catch(() => null);
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
    // Wait for the probe verdict to show a booted game. Shell games
    // (IR 1.2) boot to a STOPPED ticker at the start card — frames stay
    // 0 until play begins — so "booted" is the signal here, not frames.
    const deadline = Date.now() + 25000;
    while (Date.now() < deadline) {
      probe = await page.evaluate(() => window.__dqProbe || null).catch(() => null);
      if (probe && probe.booted) break;
      if (probe && probe.errors && probe.errors.length > 0) break;
      await sleep(500);
    }
  } catch (e) {
    push('playtest.boot_clean', 'fail', 'page load failed: ' + e.message, null);
    push('playtest.render_nonblank', 'skip', 'page never loaded', null);
    await page.close().catch(() => {});
    return { gameId, verdict: 'FAIL', checks, probe };
  }
  const coldStartMs = Date.now() - navStart;

  // --- shell start card (IR 1.2) ----------------------------------------
  // A shell game must present its start card and hold play until the
  // player starts it. The worker verifies the card, then taps Start
  // like a player would. Non-shell games skip these checks.
  const isShell = !!(probe && probe.shell);
  if (isShell) {
    const card = await page.evaluate(() => ({
      shown: !!document.querySelector('#shell-start.on'),
      title: document.getElementById('shell-start-title')?.textContent || '',
      obj: document.getElementById('shell-start-obj')?.textContent || '',
    })).catch(() => ({ shown: false, title: '', obj: '' }));
    if (card.shown && !probe.shell.started) {
      push('shell.start_card', 'pass',
        `start card shown before play: "${card.title}" / "${card.obj}"`, null);
    } else {
      push('shell.start_card', 'fail',
        `shell game booted without a gating start card (shown=${card.shown}, started=${probe.shell.started})`, null);
    }
    await page.click('#shell-start-btn').catch(() => {});
    const startDeadline = Date.now() + 8000;
    while (Date.now() < startDeadline) {
      const fresh = await page.evaluate(() => window.__dqProbe || null).catch(() => null);
      if (fresh) probe = fresh;
      if (probe && probe.shell && probe.shell.started && probe.frames > 0) break;
      await sleep(300);
    }
  }

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
        () => (document.getElementById('status')?.textContent || '') +
          (document.querySelector('#shell-start.on') ? ' shell-start-card' : '')).catch(() => '');
      if (/drag|tap|arrow|move|wasd|shell-start-card/i.test(status)) { inputReadyMs = Date.now() - navStart; break; }
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
  // Measure reward latency from the start of active bot play, not navigation.
  // Boot, census sampling, and input-prompt waits are measured separately and
  // otherwise inflate first-reward latency by several seconds.
  const playStart = Date.now();
  let firstRewardMs = null;
  let firstRewardSource = 'HUD score observation';
  let stateChanged = false;
  let winSeen = false;
  let endSeen = false;
  let lastSig = '';
  const playDeadline = Date.now() + Math.min(playSeconds, winSeconds) * 1000;
  const isCatcher = probe?.shell?.archetype === 'catcher';
  // Vertical keys are harmful to a catcher: remain in the catch lane.
  const keys = ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'];
  let ki = 0;
  while (Date.now() < playDeadline) {
    if (!isCatcher) {
      await page.keyboard.down(keys[ki % keys.length]).catch(() => {});
      await sleep(450);
      await page.keyboard.up(keys[ki % keys.length]).catch(() => {});
    } else {
      await sleep(120);
    }
    ki++;
    if (isCatcher) {
      const points = await catcherPointerPoints(page, ki % 2 ? 0.8 : 0.2);
      if (points) {
        await page.mouse.move(points.from.x, points.from.y).catch(() => {});
        await page.mouse.down().catch(() => {});
        await page.mouse.move(points.to.x, points.to.y, { steps: 8 }).catch(() => {});
        await page.mouse.up().catch(() => {});
      }
    } else if (ki % 4 === 0) {
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
      firstRewardMs: (window.__dqProbe || {}).firstRewardMs,
      endVisible: (() => { const e = document.getElementById('end'); return !!e && getComputedStyle(e).display !== 'none'; })(),
      endTitle: document.getElementById('end-title')?.textContent || '',
      shellEnd: !!document.querySelector('#shell-result.on'),
      shellTitle: document.getElementById('shell-result-title')?.textContent || '',
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
    if (firstRewardMs === null) {
      const measured = selectFirstRewardMeasurement(
        live.firstRewardMs,
        scoreOf(live.hud) > score0,
        Date.now() - playStart,
      );
      if (measured) {
        firstRewardMs = measured.ms;
        firstRewardSource = measured.source;
      }
    }
    if (live.endVisible || live.shellEnd) {
      endSeen = true;
      const title = live.shellEnd ? live.shellTitle : live.endTitle;
      if (/win|did it|complete|escaped|mended|great|hooray|whole/i.test(title) ||
          !/caught|got you|grabbed|game over|so close/i.test(title)) {
        winSeen = true;
      }
      break;
    }
  }
  if (shotsDir) {
    mkdirSync(shotsDir, { recursive: true });
    await page.screenshot({ path: `${shotsDir}/${gameId}.mid.png` }).catch(() => {});
  }
  // Shell games get an extended pursuit when the standard window saw
  // no end: continuous drag sweeps across the player's zone (the
  // drag_x scheme tracks the pointer), up to the win budget. The
  // result-card and restart checks below need an observed end; without
  // one they skip honestly.
  if (isShell && !endSeen) {
    const extDeadline = Date.now() + Math.max(0, winSeconds - playSeconds) * 1000;
    await page.mouse.move(90, 700).catch(() => {});
    await page.mouse.down().catch(() => {});
    let sweep = 0;
    while (Date.now() < extDeadline && !endSeen) {
      sweep++;
      const x = sweep % 2 === 0 ? 320 : 80;
      await page.mouse.move(x, 700, { steps: 10 }).catch(() => {});
      await sleep(650);
      const live = await page.evaluate(() => ({
        shellEnd: !!document.querySelector('#shell-result.on'),
        shellTitle: document.getElementById('shell-result-title')?.textContent || '',
        endVisible: (() => { const e = document.getElementById('end'); return !!e && getComputedStyle(e).display !== 'none'; })(),
      })).catch(() => null);
      if (live && (live.shellEnd || live.endVisible)) {
        endSeen = true;
        if (live.shellEnd && !/caught|so close/i.test(live.shellTitle)) winSeen = true;
      }
    }
    await page.mouse.up().catch(() => {});
    // winSeen/endSeen are shared with the verdict code below, so a win
    // found here counts for simulation.win_reachable automatically.
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
    push('hook.first_reward', firstRewardMs <= 15000 ? 'pass' : 'fail',
      `first reward from ${firstRewardSource} at ${firstRewardMs}ms`, { firstRewardMs, source: firstRewardSource });
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

  // Capture the submitted run id NOW, before the shell restart check
  // replaces the session (a restart wipes the verdict's lastRunId).
  let submittedRunId = null;
  if (winSeen) {
    for (let i = 0; i < 12 && !submittedRunId; i++) {
      const v = await readProbe(page).catch(() => null);
      if (v) { probe = v; submittedRunId = v.lastRunId || null; }
      if (!submittedRunId) await sleep(350);
    }
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

  // --- shell result card + in-place restart (IR 1.2) -------------------
  if (isShell) {
    if (!endSeen) {
      push('shell.result_card', 'skip', 'no run ended during bot play; result card unobserved', null);
      push('shell.restart_in_place', 'skip', 'no run ended during bot play; restart unobserved', null);
    } else {
      const cardOn = await page.evaluate(
        () => !!document.querySelector('#shell-result.on')).catch(() => false);
      push('shell.result_card', cardOn ? 'pass' : 'fail',
        cardOn ? 'result card presented in-canvas at the end of the run'
               : 'run ended without the shell result card', null);
      if (cardOn) {
        // A marker that only survives if the page does NOT reload.
        await page.evaluate(() => { window.__dqRestartMark = 'alive'; }).catch(() => {});
        const clickAt = Date.now();
        await page.click('#shell-again-btn').catch(() => {});
        let restartedMs = null;
        const rDeadline = Date.now() + 6000;
        while (Date.now() < rDeadline) {
          const st = await page.evaluate(() => ({
            mark: window.__dqRestartMark || '',
            cardShown: !!document.querySelector('#shell-start.on'),
            started: !!((window.__dqProbe || {}).shell || {}).started,
            frames: (window.__dqProbe || {}).frames || 0,
            hud: document.getElementById('score')?.textContent || '',
          })).catch(() => null);
          if (st && st.mark === 'alive' && !st.cardShown && st.started && st.frames > 5) {
            restartedMs = Date.now() - clickAt;
            break;
          }
          if (st && st.mark !== 'alive') break; // page reloaded
          await sleep(250);
        }
        if (restartedMs !== null) {
          push('shell.restart_in_place', 'pass',
            `Play again restarted in place in ${restartedMs}ms: no reload (marker survived), no start card on retry, frames advancing`,
            { restartedMs });
        } else {
          push('shell.restart_in_place', 'fail',
            'Play again did not return to in-place play within 6s (reload or start card suspected)', null);
        }
      } else {
        push('shell.restart_in_place', 'skip', 'result card missing; restart not attempted', null);
      }
    }
  }

  // --- Phase C: determinism + replay verification ---------------------
  // Determinism: two fresh pages, same seed, same scripted step-level
  // inputs — the fixed-timestep sim must land on the same final score.
  try {
    const script = [];
    for (let i = 0; i <= 7; i++) {
      script.push({ tick: 30 + i * 30, type: 'pointer', detail: `${90 + i * 30},720` });
    }
    script.push({ tick: 280, type: 'pointerup', detail: '' });
    const scriptedScore = async () => {
      const p2 = await browser.newPage({ viewport: { width: 420, height: 900 } });
      try {
        await p2.goto(`${base}/player?game=${encodeURIComponent(gameId)}&probe=1&seed=777`,
          { waitUntil: 'domcontentloaded', timeout: 30000 });
        await p2.waitForFunction(() => {
          const el = document.querySelector('#probe-result');
          if (!el) return false;
          try { return JSON.parse(el.textContent).booted === true; } catch { return false; }
        }, { timeout: 25000 });
        await p2.evaluate((s) => { window.__dqScripted = s; window.__dqScriptEnd = 330; }, script);
        const hasCard = await p2.evaluate(() => !!document.querySelector('#shell-start.on'));
        if (hasCard) await p2.click('#shell-start-btn');
        else await p2.evaluate(() => { window.__dqBeginPlay && window.__dqBeginPlay(); });
        // The runtime snapshots the score at exactly step 330; poll
        // for that snapshot (a live read would overshoot steps and
        // fake a divergence).
        const t0 = Date.now();
        let snap = null;
        while (Date.now() - t0 < 30000) {
          const v = await readProbe(p2);
          if (v && typeof v.scriptEndScore === 'number') { snap = v.scriptEndScore; break; }
          await sleep(300);
        }
        return snap;
      } finally {
        await p2.close().catch(() => {});
      }
    };
    const s1 = await scriptedScore();
    const s2 = await scriptedScore();
    if (s1 === null || s2 === null) {
      push('simulation.determinism', 'skip', 'scripted runs did not produce a readable score', { s1, s2 });
    } else if (s1 === s2) {
      push('simulation.determinism', 'pass',
        `seed 777 + identical scripted trace produced score ${s1} in both runs`, { s1, s2 });
    } else {
      push('simulation.determinism', 'fail',
        `same seed + same scripted trace diverged: run A score ${s1}, run B score ${s2}`, { s1, s2 });
    }
  } catch (e) {
    push('simulation.determinism', 'skip', `determinism probe errored: ${String(e).slice(0, 120)}`, null);
  }
  // Replay verification: when the main session won and its run was
  // submitted, replay the recorded trace and require an exact match.
  try {
    const runId = submittedRunId;
    if (!winSeen) {
      push('leaderboard.replay_verified', 'skip', 'bot did not win this session; no winning run to verify', null);
    } else if (!runId) {
      push('leaderboard.replay_verified', 'skip', 'no submitted run id observed after the win (submission may have been rejected)', null);
    } else {
      const p3 = await browser.newPage({ viewport: { width: 420, height: 900 } });
      try {
        await p3.goto(`${base}/player?game=${encodeURIComponent(gameId)}&probe=1&replay=${encodeURIComponent(runId)}`,
          { waitUntil: 'domcontentloaded', timeout: 30000 });
        let verdict = null;
        const t0 = Date.now();
        while (Date.now() - t0 < 45000) {
          const v = await readProbe(p3).catch(() => null);
          if (v && v.replay) { verdict = v.replay; break; }
          await sleep(500);
        }
        if (!verdict) {
          push('leaderboard.replay_verified', 'fail',
            `replay of ${runId} produced no verdict within 45s`, { runId });
        } else if (verdict.match) {
          push('leaderboard.replay_verified', 'pass',
            `replay of ${runId} reproduced the submitted score exactly (${verdict.actual})`, verdict);
        } else {
          push('leaderboard.replay_verified', 'fail',
            `replay of ${runId} diverged: expected ${verdict.expected}, replayed ${verdict.actual}${verdict.reason ? ' (' + verdict.reason + ')' : ''}`, verdict);
        }
      } finally {
        await p3.close().catch(() => {});
      }
    }
  } catch (e) {
    push('leaderboard.replay_verified', 'skip', `replay probe errored: ${String(e).slice(0, 120)}`, null);
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
     c.checkId === 'simulation.no_softlock' || c.checkId === 'shell.start_card' ||
     c.checkId === 'shell.restart_in_place'));
  return { gameId, verdict: failed ? 'FAIL' : 'PASS', checks, probe };
}

async function main() {
  const gameIdsArg = (arg('games', '') || '').split(',').map((s) => s.trim()).filter(Boolean);
  const obscuraCDP = process.env.OBSCURA_CDP_URL;
  if (obscuraCDP && browserName === 'webkit') {
    throw new Error('OBSCURA_CDP_URL requires --browser chromium; keep WebKit for iOS/WKWebView compatibility coverage.');
  }
  const engine = browserName === 'webkit' ? webkit : chromium;
  // Optional Obscura/CDP execution backend; default is local Playwright.
  const browser = obscuraCDP ? await chromium.connectOverCDP(obscuraCDP) : await engine.launch();
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
