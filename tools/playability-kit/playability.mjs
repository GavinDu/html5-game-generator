#!/usr/bin/env node
// Playability Kit — vendored from the advisor prototype (2026-10-23) and
// adapted to the shipped platform:
//   * the window.__dqTest snapshot hook is NATIVE in pixi-runtime.js
//     (probe mode), so no runtime patching on the normal path; route
//     patching remains only for the runtime-throw mutant self-test;
//   * the end screen is the IR 1.2 shell result card (#shell-result +
//     #shell-again-btn), not the legacy #end overlay;
//   * games start behind the shell start card, so the kit taps Start
//     like a user (#shell-start-btn);
//   * control test falls back to taps when a drag does not move the
//     player (tap-zone games), and the play loop re-presses periodically
//     so tap-to-move games receive discrete taps;
//   * the Chromium executable is resolved from PW_CHROMIUM, the local
//     Playwright cache, or /opt/pw-browsers/chromium.
// Guarantees MINIMUM playability of a
// generated game by playing it like a user would, in a real browser:
//
//   L0 load      boots clean, every asset loads, first frame is not blank
//   L1 control   the player really responds to a finger drag
//   L2 resilience rotate, input spam, offline blip: no errors, still running
//   L3 play      a "mimic the user" brain plays with intent; progress, rule
//                coverage, win reachability, end screen, restart
//   L4 perf      frame-time percentiles (report-only)
//
// Output: one report per game with a verdict and an issue list. Each issue
// carries a class (content | asset | runtime | env) and a fix hint, which
// is exactly what the repair loop needs.
//
// Usage: node playability.mjs --base http://localhost:8099 [--games a,b]
//        [--budget 40] [--mutant missing-asset|no-win|runtime-throw|dead-pointer]
//        [--out out]

import { chromium } from 'playwright-core';
import { PNG } from 'pngjs';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { heuristicBrain } from './brains/heuristic.mjs';

function resolveChromium() {
  if (process.env.PW_CHROMIUM && fs.existsSync(process.env.PW_CHROMIUM)) return process.env.PW_CHROMIUM;
  const cache = path.join(os.homedir(), '.cache', 'ms-playwright');
  try {
    for (const d of fs.readdirSync(cache).filter((x) => x.startsWith('chromium-')).sort().reverse()) {
      const p = path.join(cache, d, 'chrome-linux', 'chrome');
      if (fs.existsSync(p)) return p;
    }
  } catch { /* no cache */ }
  return '/opt/pw-browsers/chromium';
}

const arg = (n, d) => { const i = process.argv.indexOf('--' + n); return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : d; };
const base = arg('base', 'http://localhost:8099').replace(/\/$/, '');
const budgetS = Number(arg('budget', '40'));
const mutant = arg('mutant', '');
const outDir = arg('out', 'out');
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
fs.mkdirSync(outDir, { recursive: true });

// Checks whose failure means "a player would hit a broken game".
const BLOCKING = new Set(['load.clean', 'load.assets', 'render.nonblank', 'control.pointer',
  'play.win_possible', 'end.flow', 'end.restart', 'run.no_errors', 'resil.stable']);

const ISSUE_MAP = {
  'load.clean':        { cls: 'runtime', fix: 'Runtime failed to boot: check the error fingerprint; if only this game fails, regenerate its IR.' },
  'load.assets':       { cls: 'asset',   fix: 'Re-run asset generation; repair rules drop-dangling-asset / reencode-image apply.' },
  'render.nonblank':   { cls: 'content', fix: 'Stage is blank: entities off-screen or invisible; apply clamp-position, else regenerate.' },
  'control.pointer':   { cls: 'runtime', fix: 'Player ignores finger input: check player entity/controller setup and canvas pointer handling.' },
  'play.progress':     { cls: 'content', fix: 'The scripted bot made no progress. Inconclusive on its own: escalate to the LLM brain; if it also fails, the goal is unreachable or unclear (regenerate or tune difficulty).' },
  'play.win_possible': { cls: 'content', fix: 'No way to win: apply add-win-rule or regenerate the objective.' },
  'end.flow':          { cls: 'runtime', fix: 'End screen missing or empty after the game ended.' },
  'end.restart':       { cls: 'runtime', fix: 'Play again did not return to a fresh game.' },
  'run.no_errors':     { cls: 'runtime', fix: 'Uncaught error during play: file under its fingerprint; likely a runtime bug if seen on other games.' },
  'resil.stable':      { cls: 'runtime', fix: 'Game broke under rotation, input spam or a network blip.' },
};

function analyzePng(buf) {
  const png = PNG.sync.read(buf);
  const seen = new Set(); let sum = 0, sum2 = 0; const n = png.width * png.height;
  for (let i = 0; i < n; i += 7) {
    const r = png.data[i * 4], g = png.data[i * 4 + 1], b = png.data[i * 4 + 2];
    seen.add(((r >> 4) << 8) | ((g >> 4) << 4) | (b >> 4));
    const l = 0.299 * r + 0.587 * g + 0.114 * b; sum += l; sum2 += l * l;
  }
  const m = Math.ceil(n / 7), mean = sum / m;
  return { distinctColors: seen.size, lumaStd: Math.sqrt(Math.max(0, sum2 / m - mean * mean)) };
}

async function runGame(browser, id) {
  const checks = []; const issues = [];
  const add = (checkId, status, evidence, extra = {}) => checks.push({ checkId, status, evidence, ...extra });
  const ctx = await browser.newContext({ viewport: { width: 430, height: 960 }, deviceScaleFactor: 1, hasTouch: true });
  const page = await ctx.newPage();

  // ---- collectors -------------------------------------------------------
  const errors = [], warns = [], badResponses = [], failedReqs = []; const fired = new Set();
  let texturesLine = null; let seenPkg = null;
  page.on('pageerror', (e) => errors.push(String(e.message || e).split('\n')[0]));
  page.on('console', (m) => {
    const t = m.text();
    if (m.type() === 'error' && !/Failed to load resource/.test(t)) errors.push(t.split('\n')[0]);
    if (/\[RT\] (UNCAUGHT|TICK ERROR)/.test(t)) errors.push(t.slice(0, 200));
    if (/ WARN /.test(t) && !/audio play blocked/.test(t)) warns.push(t.replace(/^\[[^\]]*\]\s*/, '').slice(0, 160));
    const f = /rule '([^']+)' fired/.exec(t); if (f) fired.add(f[1]);
    const tx = /textures loaded: (\d+)\/(\d+)/.exec(t); if (tx) texturesLine = [Number(tx[1]), Number(tx[2])];
  });
  page.on('requestfailed', (r) => { if (!/favicon/.test(r.url())) failedReqs.push(r.url()); });
  page.on('response', (r) => { if (r.status() >= 400 && !/favicon/.test(r.url())) badResponses.push(`${r.status()} ${r.url().replace(base, '')}`); });

  await page.addInitScript(() => {
    window.__ft = []; let last = 0;
    const loop = (t) => { if (last) window.__ft.push(t - last); last = t; requestAnimationFrame(loop); };
    requestAnimationFrame(loop);
  });

  // ---- mutants that patch the served runtime (self-tests only) -----------
  if (mutant === 'runtime-throw') {
    await page.route('**/web/pixi-runtime.js', async (route) => {
      const r = await route.fetch(); let body = await r.text();
      const ANCHOR = 'probeState.booted = true;';
      if (body.split(ANCHOR).length !== 2) throw new Error('hook anchor not found exactly once; runtime changed');
      body = body.replace(ANCHOR, ANCHOR + "\n  setTimeout(() => { throw new Error('mutant: render exploded'); }, 3000);");
      await route.fulfill({ response: r, body });
    });
  }
  await page.route('**/v1/games/*/package', async (route) => {
    const r = await route.fetch(); const j = await r.json();
    if (mutant === 'missing-asset') {
      const a = (j.assets || []).find((x) => x.kind === 'image');
      if (a) { a.url = '/v1/games/x/assets/__missing__'; a.uri = a.url; }
    }
    if (mutant === 'no-win') {
      for (const rule of j.ir.rules) rule.actions = rule.actions.filter((a) => !(a.type === 'end_game' && a.value === true));
    }
    seenPkg = j;
    await route.fulfill({ response: r, json: j });
  });

  // ---- L0 load ------------------------------------------------------------
  const nav0 = Date.now(); let ttp = null;
  await page.goto(`${base}/player?game=${encodeURIComponent(id)}&probe=1`, { waitUntil: 'load', timeout: 30000 }).catch(() => {});
  if (mutant === 'dead-pointer') await page.addStyleTag({ content: '#stage canvas{pointer-events:none}' }).catch(() => {});
  for (let i = 0; i < 40; i++) {
    const f = await page.evaluate(() => window.__dqTest ? window.__dqTest.snapshot().frames : -1).catch(() => -1);
    if (f > 2) { ttp = Date.now() - nav0; break; }
    // The shell start card gates the ticker: a user taps Start, so do we.
    const startBtn = page.locator('#shell-start-btn');
    if (await startBtn.isVisible().catch(() => false)) await startBtn.click().catch(() => {});
    await sleep(500);
  }
  if (ttp === null) {
    add('load.clean', 'fail', `never reached a running frame in 20 s; errors: ${errors.slice(0, 2).join(' | ') || 'none'}`);
    return finish();
  }
  await sleep(1200);
  const snap = () => page.evaluate(() => window.__dqTest.snapshot()).catch(() => null);
  const s0 = await snap();
  const toClient = async (lx, ly) => {
    const b = await page.locator('#stage canvas').boundingBox();
    return { x: b.x + lx * b.width / s0.world.w, y: b.y + ly * b.height / s0.world.h };
  };
  const shot = async (name) => {
    const buf = await page.locator('#stage canvas').screenshot().catch(() => null);
    if (buf) fs.writeFileSync(`${outDir}/${id}${mutant ? '.' + mutant : ''}.${name}.png`, buf);
    return buf;
  };

  add('load.clean', errors.length === 0 ? 'pass' : 'fail',
    errors.length === 0 ? `running frame after ${ttp} ms, 0 errors` : `errors during load: ${errors.slice(0, 2).join(' | ')}`, { ttpMs: ttp });
  const assetProblems = [...badResponses.filter((u) => /\/assets\/|sprites|audio/.test(u)), ...failedReqs,
    ...warns.filter((w) => /asset/i.test(w))];
  const texBad = texturesLine && texturesLine[0] < texturesLine[1];
  add('load.assets', assetProblems.length === 0 && !texBad ? 'pass' : 'fail',
    assetProblems.length === 0 && !texBad ? `all assets loaded${texturesLine ? ` (${texturesLine[0]}/${texturesLine[1]} textures)` : ''}`
      : `asset problems: ${[...assetProblems, texBad ? `textures ${texturesLine[0]}/${texturesLine[1]}` : ''].filter(Boolean).slice(0, 3).join(' | ')}`);
  const buf0 = await shot('ready');
  const px = buf0 ? analyzePng(buf0) : null;
  add('render.nonblank', px && px.distinctColors >= 12 && px.lumaStd >= 5 ? 'pass' : 'fail',
    px ? `${px.distinctColors} distinct colors, luma std ${px.lumaStd.toFixed(1)}` : 'could not capture the canvas', px || {});

  // ---- L1 control ---------------------------------------------------------
  const controls = { movesX: false, movesY: false };
  if (!s0.player) {
    add('control.pointer', 'fail', 'no player entity in the running game');
  } else {
    const p0 = s0.player; const W = s0.world.w, H = s0.world.h;
    const a = await toClient(p0.x, p0.y); await page.mouse.move(a.x, a.y); await page.mouse.down();
    const b = await toClient(Math.min(W - 30, p0.x + 90), Math.max(30, p0.y - 90));
    await page.mouse.move(b.x, b.y, { steps: 10 }); await sleep(900);
    let s1 = await snap(); await page.mouse.up();
    let dx = (s1?.player?.x ?? p0.x) - p0.x, dy = (s1?.player?.y ?? p0.y) - p0.y;
    let style = 'drag';
    if (Math.abs(dx) <= 8 && Math.abs(dy) <= 8) {
      // Tap-zone games (tap a side to switch lanes) ignore drags: tap the
      // opposite half from the player, like a user figuring it out.
      style = 'tap';
      const tapAt = await toClient(p0.x < W / 2 ? W * 0.8 : W * 0.2, p0.y);
      await page.mouse.click(tapAt.x, tapAt.y); await sleep(700);
      s1 = await snap();
      dx = (s1?.player?.x ?? p0.x) - p0.x; dy = (s1?.player?.y ?? p0.y) - p0.y;
    }
    controls.movesX = Math.abs(dx) > 8; controls.movesY = Math.abs(dy) > 8;
    add('control.pointer', controls.movesX || controls.movesY ? 'pass' : 'fail',
      `via ${style} input the player moved dx=${dx.toFixed(0)}, dy=${dy.toFixed(0)} (axes: ${controls.movesX ? 'x' : ''}${controls.movesY ? 'y' : ''}${!controls.movesX && !controls.movesY ? 'none' : ''})`, { dx, dy, style });
  }

  // ---- L2 resilience ------------------------------------------------------
  {
    const e0 = errors.length; const problems = [];
    const f0 = (await snap())?.frames ?? 0;
    await page.setViewportSize({ width: 960, height: 430 }); await sleep(1300);
    const f1 = (await snap())?.frames ?? 0; if (f1 - f0 < 2) problems.push('stopped rendering after rotation');
    await page.setViewportSize({ width: 430, height: 960 }); await sleep(500);
    for (let i = 0; i < 60; i++) {
      const c = await toClient(30 + Math.random() * 330, 60 + Math.random() * 700);
      await page.mouse.move(c.x, c.y); await page.mouse.down(); await page.mouse.up();
    }
    const f2 = (await snap())?.frames ?? 0; await sleep(700); const f3 = (await snap())?.frames ?? 0;
    if (f3 - f2 < 2) problems.push('froze after 60 rapid taps');
    await ctx.setOffline(true); await sleep(2500); await ctx.setOffline(false);
    const f4 = (await snap())?.frames ?? 0; await sleep(700); const f5 = (await snap())?.frames ?? 0;
    if (f5 - f4 < 2) problems.push('froze during a network blip');
    if (errors.length > e0) problems.push(`new errors: ${errors.slice(e0, e0 + 2).join(' | ')}`);
    add('resil.stable', problems.length === 0 ? 'pass' : 'fail',
      problems.length === 0 ? 'survived rotation, 60 rapid taps and a 2.5 s offline blip' : problems.join('; '));
  }

  // ---- L3 play (mimic the user) --------------------------------------------
  const brain = heuristicBrain();
  const timeline = []; let last = await snap(); const score0 = last?.score ?? 0; const hp0 = last?.health ?? 0;
  let firedAtStart = fired.size; let lastMoveAt = Date.now(), lastScoreAt = Date.now(), lastFired = fired.size; let stuckMs = 0;
  let lastPos = last?.player ? { x: last.player.x, y: last.player.y } : null; let tick = 0; let midShot = false;
  await page.evaluate(() => { window.__ft.length = 0; });
  const playStart = Date.now();
  if (last?.player) {
    const c = await toClient(last.player.x, last.player.y); await page.mouse.move(c.x, c.y); await page.mouse.down();
  }
  while (Date.now() - playStart < budgetS * 1000) {
    const s = await snap(); if (!s) break; last = s;
    if (s.ended) break;
    const tgt = brain.decide(s, controls);
    if (tgt) {
      const c = await toClient(tgt.x, tgt.y); await page.mouse.move(c.x, c.y, { steps: 2 });
      if (tick % 10 === 9) { await page.mouse.up().catch(() => {}); await page.mouse.down().catch(() => {}); }
    }
    if (++tick % 8 === 0) timeline.push({ t: +((Date.now() - playStart) / 1000).toFixed(1), score: s.score, health: s.health });
    if (s.player && lastPos && Math.hypot(s.player.x - lastPos.x, s.player.y - lastPos.y) > 5) { lastMoveAt = Date.now(); lastPos = { x: s.player.x, y: s.player.y }; }
    if (s.score !== (timeline.at(-2)?.score ?? score0)) lastScoreAt = Date.now();
    if (fired.size !== lastFired) { lastFired = fired.size; lastScoreAt = Date.now(); }
    stuckMs = Math.max(stuckMs, Date.now() - Math.max(lastMoveAt, lastScoreAt));
    if (!midShot && Date.now() - playStart > budgetS * 500) { midShot = true; await shot('mid'); }
    await sleep(110);
  }
  await page.mouse.up().catch(() => {});
  const fin = (await snap()) || last;
  const playedS = (Date.now() - playStart) / 1000;
  const progressed = fin.score > score0 || fin.health !== hp0 || fired.size > firedAtStart || fin.ended;
  add('play.progress', progressed ? 'pass' : 'fail',
    progressed ? `in ${playedS.toFixed(0)} s: score ${score0}→${fin.score}, health ${hp0}→${fin.health}, ${fired.size} rules fired, longest idle ${(stuckMs / 1000).toFixed(1)} s`
      : `no score, health or rule change in ${playedS.toFixed(0)} s of purposeful play`, { timeline, longestIdleS: +(stuckMs / 1000).toFixed(1) });

  const rules = seenPkg?.ir?.rules || [];
  const winRule = rules.some((r) => r.actions.some((a) => a.type === 'end_game' && (a.value === true || a.value === 'true' || a.value === 1)));
  if (fin.ended && fin.won) add('play.win_possible', 'pass', `observed a win at ${fin.score} points`, { confidence: 'observed' });
  else if (!winRule) add('play.win_possible', 'fail', 'no rule in the IR can end the game as a win: unwinnable');
  else if (progressed) add('play.win_possible', 'pass', `win rule exists and the bot made progress (score ${fin.score}); win not observed within ${budgetS} s`, { confidence: 'inferred' });
  else add('play.win_possible', 'fail', 'win rule exists but the player could not make progress toward it');
  const ruleIds = rules.map((r) => r.id); const never = ruleIds.filter((r) => !fired.has(r));
  add('info.rule_coverage', 'info', `${ruleIds.length - never.length}/${ruleIds.length} rules fired${never.length ? '; never fired: ' + never.join(', ') : ''}`, { never });

  // ---- end flow + restart ---------------------------------------------------
  if (fin.ended) {
    await shot('end');
    const dom = await page.evaluate(() => {
      const e = document.getElementById('shell-result');
      return { vis: !!e && e.classList.contains('on'), title: document.getElementById('shell-result-title')?.textContent || '', sub: document.getElementById('shell-result-sub')?.textContent || '', btn: !!document.getElementById('shell-again-btn') };
    });
    add('end.flow', dom.vis && dom.title && dom.btn ? 'pass' : 'fail',
      dom.vis ? `result card shows “${dom.title}” / “${dom.sub}”, play-again ${dom.btn ? 'present' : 'MISSING'}` : 'game ended but the shell result card is not visible');
    await page.click('#shell-again-btn').catch(() => {});
    let again = null; for (let i = 0; i < 30; i++) { again = await snap(); if (again && !again.ended && again.frames > 2) break; await sleep(500); }
    add('end.restart', again && !again.ended && again.score === 0 ? 'pass' : 'fail',
      again ? `after Play again: score ${again.score}, ended=${again.ended}` : 'game did not come back after Play again');
  } else {
    add('end.flow', 'skip', `game did not end within ${budgetS} s, so the end screen was not exercised`);
    add('end.restart', 'skip', 'needs an ended game');
  }

  // ---- L4 perf (report-only) + final error gate -----------------------------
  const ft = await page.evaluate(() => [...window.__ft].sort((a, b) => a - b)).catch(() => []);
  if (ft.length > 5) {
    const q = (f) => ft[Math.min(ft.length - 1, Math.floor(ft.length * f))];
    add('perf.frames', 'info', `p50 ${q(0.5).toFixed(0)} ms, p95 ${q(0.95).toFixed(0)} ms, ${(100 * ft.filter((x) => x > 33.4).length / ft.length).toFixed(0)}% over 33 ms (software-rendered: relative only)`,
      { p50: q(0.5), p95: q(0.95) });
  }
  add('run.no_errors', errors.length === 0 ? 'pass' : 'fail',
    errors.length === 0 ? 'no uncaught errors or runtime error logs' : `${errors.length} error(s): ${[...new Set(errors)].slice(0, 2).join(' | ')}`);
  if (warns.length) add('run.warnings', 'warn', [...new Set(warns)].slice(0, 3).join(' | '));
  return finish();

  async function finish() {
    for (const c of checks) {
      if (c.status === 'fail' || c.status === 'warn') {
        const m = ISSUE_MAP[c.checkId] || { cls: 'content', fix: 'Review the evidence.' };
        issues.push({ check: c.checkId, severity: BLOCKING.has(c.checkId) && c.status === 'fail' ? 'blocker' : (c.checkId === 'play.progress' ? 'major' : 'minor'), class: m.cls, evidence: c.evidence, fixHint: m.fix });
      }
    }
    const verdict = issues.some((i) => i.severity === 'blocker') ? 'BROKEN' : issues.length ? 'NEEDS_REVIEW' : 'PLAYABLE';
    await ctx.close().catch(() => {});
    return { gameId: id, mutant: mutant || null, verdict, checks, issues };
  }
}

const feed = await (await fetch(base + '/v1/feed')).json();
const ids = (arg('games', '') || '').split(',').filter(Boolean);
const games = ids.length ? ids : (feed.games || feed).map((g) => g.game_id || g.id);
const browser = await chromium.launch({ executablePath: resolveChromium(),
  args: ['--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist', '--no-sandbox'] });
const results = [];
for (const g of games) results.push(await runGame(browser, g));
await browser.close();
fs.writeFileSync(`${outDir}/report${mutant ? '.' + mutant : ''}.json`, JSON.stringify(results, null, 1));
for (const r of results) {
  console.log(`\n${r.verdict.padEnd(9)} ${r.gameId}${r.mutant ? '  [mutant: ' + r.mutant + ']' : ''}`);
  for (const c of r.checks) console.log(`  ${c.status.padEnd(5)} ${c.checkId.padEnd(18)} ${c.evidence}`);
}
process.exit(results.some((r) => r.verdict === 'BROKEN') ? 1 : 0);
