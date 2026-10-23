#!/usr/bin/env node
// Score browser evidence with per-game policies and optional AI review JSON.
// Usage: node score-report.mjs playtest-report.json [ai-review.json|-] [out.json]
//        [--pack <dir>] [--shots <dir>]
//        [--check-contracts] [--write-contracts]   (contract modes ignore the report args)
//
// Scorecard fusion (Phase D): the scorer receives graded measurements
// from the probe verdict (jank, p95, dead-input rate, first-input /
// first-reward timings, runtime errors); AI reviews may arrive as
// multiple blind passes which are merged here (disagreement > 15 on
// a dimension marks it contested and takes the median when a third
// pass exists); judge provenance is stamped into the output; --pack
// emits per-game evidence packs whose judge view contains
// observations only — no check statuses, no scores.
import { readFile, readdir, writeFile, mkdir, copyFile } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { scoreGame, DEFAULT_RULES } from '../../packages/game-quality-score/index.mjs';

const repoRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const argv = process.argv.slice(2);
const optVal = (name) => {
  const i = argv.indexOf(name);
  return i >= 0 ? argv[i + 1] : null;
};
const packDir = optVal('--pack');
const shotsDir = optVal('--shots');
const kitPath = optVal('--kit');
const positional = argv.filter((a, i) => !a.startsWith('--') && argv[i - 1] !== '--pack' && argv[i - 1] !== '--shots' && argv[i - 1] !== '--suggestions-md' && argv[i - 1] !== '--kit');

// Playability Kit verdicts (optional): when a kit report is supplied,
// its per-game verdict gates the scorecard — BROKEN is not scored,
// NEEDS_REVIEW caps the composite (see the scoring package, schema 1.3).
const kitVerdicts = {};
if (kitPath) {
  try {
    const kitReport = JSON.parse(readFileSync(kitPath, 'utf8'));
    for (const entry of Array.isArray(kitReport) ? kitReport : []) {
      if (entry && entry.gameId && entry.verdict) kitVerdicts[entry.gameId] = entry.verdict;
    }
  } catch (err) {
    console.error(`warning: could not read kit report ${kitPath}: ${err.message}`);
  }
}

// ---------- IR-grounded quality contracts -------------------------------
// The generated block of a game's quality contract is DERIVED from its
// Game IR experience block — the authoritative statement of what the
// game is. Human fields (audience, fairness, focus, core-loop prose)
// stay human, but prose that contradicts the generated facts is a
// finding, not an opinion.
function contractFromIR(ir) {
  const exp = ir?.experience || {};
  return {
    archetype: exp.archetype || null,
    controlsScheme: exp.controls?.scheme || null,
    objective: exp.objective ? { kind: exp.objective.kind, target: exp.objective.target, label: exp.objective.label } : null,
    failure: exp.failure ? { mode: exp.failure.mode, health: exp.failure.health ?? null, invulnerableMs: exp.failure.invulnerableMs ?? null } : null,
    targetSessionSec: exp.pacing?.targetSessionSec ?? null,
    firstRewardWithinSec: exp.pacing?.firstRewardWithinSec ?? null,
    milestones: Array.isArray(exp.milestones) ? exp.milestones.length : 0,
    scenes: Array.isArray(ir?.scenes) ? ir.scenes.length : 0,
  };
}
function contractFindings(rules, ir) {
  const findings = [];
  const gen = contractFromIR(ir);
  const existing = rules?.gameplayContract?.generated;
  if (!existing) {
    findings.push('gameplayContract.generated is missing (run --write-contracts)');
  } else if (JSON.stringify(existing) !== JSON.stringify(gen)) {
    findings.push('gameplayContract.generated drifts from the IR (run --write-contracts)');
  }
  const prose = [...(rules?.gameplayContract?.coreLoop || []), ...(rules?.gameplayContract?.fairness || [])].join(' ').toLowerCase();
  if (/miss limit|miss-limit|limited misses/.test(prose) && (gen.failure?.mode === 'gentle' || gen.failure?.mode === 'none')) {
    findings.push(`core-loop prose claims a miss limit but the IR failure mode is "${gen.failure?.mode}"`);
  }
  if (/\brounds?\b/.test(prose) && gen.scenes < 2) {
    findings.push('core-loop prose mentions rounds but the IR has a single scene and no round structure');
  }
  return { generated: gen, findings };
}
async function inventoryGameDirs() {
  const inventoryRoot = resolve(repoRoot, 'inventory');
  const entries = await readdir(inventoryRoot, { withFileTypes: true });
  return entries.filter(e => e.isDirectory()).map(e => join(inventoryRoot, e.name));
}
async function readJsonIfPresent(path) {
  try { return JSON.parse(await readFile(path, 'utf8')); }
  catch (error) { if (error?.code === 'ENOENT') return null; throw error; }
}

if (argv.includes('--check-contracts') || argv.includes('--write-contracts')) {
  const write = argv.includes('--write-contracts');
  let bad = 0;
  for (const dir of await inventoryGameDirs()) {
    const rulesPath = join(dir, 'quality-rules.json');
    const rules = await readJsonIfPresent(rulesPath);
    if (!rules) continue;
    const ir = await readJsonIfPresent(join(dir, 'game-ir.json'));
    if (!ir) continue;
    const { generated, findings } = contractFindings(rules, ir);
    if (write) {
      rules.gameplayContract = rules.gameplayContract || {};
      rules.gameplayContract.generated = generated;
      await writeFile(rulesPath, JSON.stringify(rules, null, 2) + '\n');
      console.log(`${dir}: generated contract written (${findings.length} prose/drift finding(s) remain)`);
      for (const f of findings) if (!f.includes('generated')) { console.log(`  - ${f}`); bad++; }
    } else {
      if (findings.length) { bad++; console.log(`${dir}:`); for (const f of findings) console.log(`  - ${f}`); }
      else console.log(`${dir}: contract matches IR`);
    }
  }
  process.exit(bad ? 1 : 0);
}

// ---------- multi-pass review merge --------------------------------------
// Passes are blind: the merge is the first place they meet. A spread
// over 15 on a dimension marks it contested; with three passes the
// median stands, with two the mean stands (flagged for a third pass).
function mergePasses(review) {
  if (!review || !Array.isArray(review.passes) || !review.passes.length) return review;
  const byId = new Map();
  for (const pass of review.passes) {
    for (const m of pass.metrics || []) {
      if (!byId.has(m.id)) byId.set(m.id, []);
      byId.get(m.id).push(m);
    }
  }
  const metrics = [];
  for (const [id, entries] of byId) {
    const scores = entries.map(e => e.score).filter(Number.isFinite).sort((a, b) => a - b);
    if (!scores.length) continue;
    const contested = scores.length >= 2 && scores[scores.length - 1] - scores[0] > 15;
    const score = scores.length >= 3 ? scores[1] : Math.round(scores.reduce((a, b) => a + b, 0) / scores.length * 10) / 10;
    const medianEntry = entries.reduce((best, e) => (Math.abs(e.score - score) < Math.abs(best.score - score) ? e : best), entries[0]);
    metrics.push({ id, score, rationale: medianEntry.rationale, evidence: medianEntry.evidence, ...(contested ? { contested: true } : {}) });
  }
  const seen = new Set();
  const improvementSuggestions = [];
  for (const pass of review.passes) {
    for (const s of pass.improvementSuggestions || []) {
      if (s && !seen.has(s.metricId)) { seen.add(s.metricId); improvementSuggestions.push(s); }
    }
  }
  return { metrics, improvementSuggestions, judge: review.judge || null, passes: review.passes.length };
}

// ---------- measured-evidence suggestions ---------------------------------
// The AI reviewer judges creative dimensions; OBJECTIVE dimensions
// below the floor (or unscored) get their suggestions here, derived
// deterministically from the measurements themselves — never
// invented. Together with the reviewer's suggestions this closes the
// loop the reviewer contract promises: every weak or missing metric
// leaves with a concrete, prioritized action (quality.missingSuggestions
// is recomputed after the merge).
function measuredSuggestions(game, quality, measurements) {
  const out = [];
  const dim = (id) => quality.dimensions.find(d => d.id === id);
  const checks = game.checks || [];
  const st = (id) => checks.find(c => c.checkId === id)?.status ?? null;
  const floor = quality.thresholds.minimumMetricScore;
  const weak = (id) => { const d = dim(id); return d && (d.score === null || d.score < floor); };

  if (weak('performance')) {
    const cold = checks.find(c => c.checkId === 'budgets.cold_start');
    const ms = cold?.actual?.coldStartMs;
    const bits = [];
    if (Number.isFinite(ms)) bits.push(`cold start measured ${ms}ms`);
    if (Number.isFinite(measurements.jankRate)) bits.push(`jank rate ${(measurements.jankRate * 100).toFixed(1)}%`);
    if (Number.isFinite(measurements.p95FrameMs)) bits.push(`p95 frame ${measurements.p95FrameMs}ms`);
    out.push({
      metricId: 'performance', priority: 'high', source: 'measured',
      problem: `Performance scored ${dim('performance').score ?? 'unscored'} (${bits.join('; ') || 'insufficient measurement'}).`,
      suggestion: 'Profile the boot path first: keep first-frame work minimal, defer non-critical asset decode until after the start card, and re-measure on a device-class target — shared CI runners inflate cold start, so confirm on the live deployment before cutting content. If jank is the driver, reduce simultaneous particles/full-screen layers (the render-cost drivers) before touching gameplay.',
    });
  }
  if (weak('input_responsiveness')) {
    const target = measurements.firstRewardTargetSec;
    const bits = [];
    if (Number.isFinite(measurements.firstRewardMs)) bits.push(`first reward at ${(measurements.firstRewardMs / 1000).toFixed(1)}s${target ? ` vs the declared ${target}s target` : ''}`);
    if (Number.isFinite(measurements.deadInputRate)) bits.push(`dead-input rate ${(measurements.deadInputRate * 100).toFixed(0)}%`);
    if (!bits.length) bits.push('no input/reward timing was measured in the playtest session');
    out.push({
      metricId: 'input_responsiveness', priority: 'high', source: 'measured',
      problem: `Input responsiveness scored ${dim('input_responsiveness').score ?? 'unscored'} (${bits.join('; ')}).`,
      suggestion: 'Bring the first reward inside the declared firstRewardWithinSec: spawn the first collectible on a guaranteed early, reachable trajectory at session start, and audit input handling for dead zones (inputs with no state change). Re-run the playtest so the timings are measured, not inferred.',
    });
  }
  if (weak('playability')) {
    const causes = [];
    if (st('simulation.win_reachable') !== 'pass') causes.push('the playtest bot never observed a win');
    for (const id of ['shell.start_card', 'shell.result_card', 'shell.restart_in_place']) if (st(id) === 'fail') causes.push(`${id} failed`);
    if (st('simulation.determinism') === 'fail') causes.push('determinism check failed');
    if (st('leaderboard.replay_verified') === 'fail') causes.push('replay verification failed');
    out.push({
      metricId: 'playability', priority: 'high', source: 'measured',
      problem: `Playability scored ${dim('playability').score} (${causes.join('; ') || 'gate evidence incomplete'}).`,
      suggestion: 'Compare the objective economics with the session target (spawn rate × catch probability × session length): if the win is unreachable in the target session, ease the pacing ramp or raise early spawn density by one step, then re-run the playtest — the win must be bot-observable before it can be scored.',
    });
  }
  if (weak('reliability')) {
    out.push({
      metricId: 'reliability', priority: 'high', source: 'measured',
      problem: `Reliability scored ${dim('reliability').score} (${measurements.errorCount ?? 0} runtime errors, ${measurements.warnClasses ?? 0} warning classes in the session).`,
      suggestion: 'Fix runtime errors before any polish: reproduce from the telemetry incident fingerprint, repair the root cause in the IR/runtime, and confirm a clean session in the next playtest.',
    });
  }
  return out;
}

function suggestionsMarkdown(report) {
  const lines = ['# Improvement suggestions — AI reviewer + measured evidence', ''];
  for (const game of report.games) {
    const q = game.quality;
    if (!q) continue;
    lines.push(`## ${game.title || game.gameId} — overall ${q.overallScore ?? 'incomplete'}, measured ${q.subscores.measured.score ?? '—'}, creative ${q.subscores.creative.score ?? '—'}`);
    lines.push('');
    const all = q.improvementSuggestions || [];
    if (!all.length) lines.push('_No suggestions — every metric is at or above its floor._', '');
    for (const s of all) {
      const d = q.dimensions.find(x => x.id === s.metricId);
      lines.push(`- **[${s.priority || 'medium'}] ${s.metricId}** (score ${d?.score ?? 'unscored'}, source: ${s.source || 'ai-review'}) — ${s.problem || ''}`);
      lines.push(`  → ${s.suggestion}`);
    }
    if ((q.missingSuggestions || []).length) {
      lines.push(`- ⚠️ Still missing suggestions for: ${q.missingSuggestions.map(m => m.metricId).join(', ')}`);
    }
    lines.push('');
  }
  return lines.join('\n');
}

// ---------- scoring --------------------------------------------------------
const [reportPath, reviewPath, outPath] = positional;
if (!reportPath) {
  console.error('Usage: node score-report.mjs playtest-report.json [ai-review.json|-] [out.json] [--pack dir] [--shots dir]');
  process.exit(2);
}
const reportText = await readFile(resolve(reportPath), 'utf8');
if (!reportText.trim()) {
  throw new Error('playtest report is empty; the browser worker may have timed out before emitting its final JSON report');
}
const report = JSON.parse(reportText);
const reviewsRaw = reviewPath && reviewPath !== '-' ? JSON.parse(await readFile(resolve(reviewPath), 'utf8')) : {};
if (!Array.isArray(report.games)) throw new Error('playtest report must contain a games array');

async function findGameRules(game) {
  if (game.rulesPath) return JSON.parse(await readFile(resolve(repoRoot, game.rulesPath), 'utf8'));
  const inventoryRoot = resolve(repoRoot, 'inventory');
  const entries = await readdir(inventoryRoot, { withFileTypes: true });
  const id = game.gameId || game.id;
  for (const entry of entries.filter(e => e.isDirectory())) {
    const gameDir = join(inventoryRoot, entry.name);
    const manifest = await readJsonIfPresent(join(gameDir, 'manifest.json'));
    if (!manifest || (manifest.game_id !== id && manifest.title !== game.title)) continue;
    const rules = await readJsonIfPresent(join(gameDir, 'quality-rules.json'));
    if (rules) return rules;
  }
  return (await readJsonIfPresent(resolve(repoRoot, 'config/game-quality/default.json'))) || DEFAULT_RULES;
}
async function findGameIR(game) {
  const inventoryRoot = resolve(repoRoot, 'inventory');
  const entries = await readdir(inventoryRoot, { withFileTypes: true });
  const id = game.gameId || game.id;
  for (const entry of entries.filter(e => e.isDirectory())) {
    const gameDir = join(inventoryRoot, entry.name);
    const manifest = await readJsonIfPresent(join(gameDir, 'manifest.json'));
    if (!manifest || (manifest.game_id !== id && manifest.title !== game.title)) continue;
    return readJsonIfPresent(join(gameDir, 'game-ir.json'));
  }
  return null;
}

for (const game of report.games) {
  const rules = await findGameRules(game);
  const rawReview = reviewsRaw[game.gameId] || reviewsRaw[game.title] || null;
  const review = mergePasses(rawReview);
  const probe = game.probe || {};
  const measurements = {
    jankRate: probe.jankRate ?? undefined,
    p95FrameMs: probe.p95FrameMs ?? undefined,
    deadInputRate: probe.deadInputRate ?? undefined,
    firstInputMs: probe.firstInputMs ?? undefined,
    firstRewardMs: probe.firstRewardMs ?? undefined,
    firstRewardTargetSec: rules?.gameplayContract?.generated?.firstRewardWithinSec ?? undefined,
    errorCount: Array.isArray(probe.errors) ? probe.errors.length : undefined,
    warnClasses: Number.isFinite(probe.warnCount) ? probe.warnCount : undefined,
  };
  game.quality = scoreGame(game.checks || [], review, rules, measurements, { kitVerdict: kitVerdicts[game.gameId] || null });
  game.quality.rulesFile = rules.profile === 'default'
    ? 'config/game-quality/default.json'
    : `inventory/<game-id>/quality-rules.json`;
  // Merge reviewer + measured-evidence suggestions; a metric counts
  // as covered when either source produced a concrete suggestion.
  {
    const judgeSuggestions = (game.quality.improvementSuggestions || []).map(s => ({ source: 'ai-review', ...s }));
    const measured = measuredSuggestions(game, game.quality, measurements);
    const covered = new Set([...judgeSuggestions, ...measured].map(s => s.metricId));
    game.quality.improvementSuggestions = [...judgeSuggestions, ...measured];
    game.quality.missingSuggestions = (game.quality.reviewRequests || [])
      .filter(r => !covered.has(r.metricId))
      .map(r => ({ metricId: r.metricId, score: r.currentScore }));
  }
  if (review && review.judge) {
    game.quality.provenance = { judge: review.judge, passes: review.passes || 1 };
  }

  if (packDir) {
    // Evidence pack: the judge view carries observations only.
    const ir = await findGameIR(game);
    const dir = join(resolve(packDir), game.gameId);
    await mkdir(dir, { recursive: true });
    let shotName = null;
    if (shotsDir) {
      try {
        await copyFile(join(resolve(shotsDir), `${game.gameId}.png`), join(dir, 'end.png'));
        shotName = 'end.png';
      } catch { /* no screenshot for this game */ }
    }
    const judgeView = {
      gameId: game.gameId,
      title: game.title || rules.title || game.gameId,
      audience: rules?.gameplayContract?.audience || null,
      reviewFocus: rules?.reviewFocus || [],
      experience: ir?.experience || null,
      intent: ir?.game?.description || null,
      audio: ir ? {
        assets: (ir.assets || []).filter(a => a.kind === 'audio').map(a => a.id),
        soundEvents: (ir.rules || [])
          .filter(r => (r.actions || []).some(a => a.type === 'play_sound'))
          .map(r => ({ on: r.when?.event, sounds: r.actions.filter(a => a.type === 'play_sound').map(a => a.assetId) })),
      } : null,
      observations: {
        census: probe.census || null,
        hud: probe.hud || null,
        status: probe.status || null,
        events: Array.isArray(probe.events) ? probe.events.slice(-25) : [],
        framesRendered: probe.frames ?? null,
      },
      screenshots: shotName ? [shotName] : [],
    };
    const viewText = JSON.stringify(judgeView, null, 2) + '\n';
    await writeFile(join(dir, 'judge-view.json'), viewText);
    const packHash = createHash('sha256').update(viewText).digest('hex').slice(0, 16);
    await writeFile(join(dir, 'pack.json'), JSON.stringify({ gameId: game.gameId, packHash, files: ['judge-view.json', ...(shotName ? [shotName] : [])] }, null, 2) + '\n');
    if (game.quality.provenance) game.quality.provenance.packHash = packHash;
    game.packHash = packHash;
  }
}
const output = JSON.stringify(report, null, 2) + '\n';
if (outPath) await writeFile(resolve(outPath), output);
else process.stdout.write(output);
const suggestionsPath = optVal('--suggestions-md');
if (suggestionsPath) {
  await writeFile(resolve(suggestionsPath), suggestionsMarkdown(report));
}
