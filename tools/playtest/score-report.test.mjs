// Integration tests for score-report.mjs (scorecard fusion, Phase D):
// multi-pass blind merge, provenance, and evidence-pack emission.
import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, writeFile, readFile, access } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import { METRICS } from '../../packages/game-quality-score/index.mjs';

const run = promisify(execFile);
const script = new URL('./score-report.mjs', import.meta.url).pathname;

const checks = [
  { checkId: 'playtest.boot_clean', status: 'pass', evidence: 'clean' },
  { checkId: 'playtest.render_nonblank', status: 'pass', evidence: 'visible' },
  { checkId: 'simulation.no_softlock', status: 'pass', evidence: 'progressed' },
  { checkId: 'simulation.win_reachable', status: 'pass', evidence: 'bot won' },
  { checkId: 'hook.time_to_input', status: 'pass' },
  { checkId: 'hook.first_reward', status: 'pass' },
  { checkId: 'budgets.cold_start', status: 'pass', actual: { coldStartMs: 900 } },
];
const aiIds = METRICS.filter(m => m.source === 'ai').map(m => m.id);
const pass = (score, plotScore) => ({
  metrics: aiIds.map(id => ({
    id, score: id === 'plot' && plotScore !== undefined ? plotScore : score,
    rationale: 'Grounded rationale.', evidence: 'Observed in play.',
  })),
  improvementSuggestions: [],
});
const report = {
  games: [{
    gameId: 'sample-rainbow-bear', title: "Bobo's Rainbow", checks,
    probe: {
      status: 'won', census: 'bear=bobo', errors: [], warnCount: 0,
      jankRate: 0.02, p95FrameMs: 21, deadInputRate: 0, firstInputMs: 900, firstRewardMs: 2600,
      events: [{ type: 'run_end' }],
    },
  }],
};

async function runScorer(review) {
  const dir = await mkdtemp(join(tmpdir(), 'dq-score-'));
  const reportPath = join(dir, 'report.json');
  const reviewPath = join(dir, 'review.json');
  const outPath = join(dir, 'out.json');
  await writeFile(reportPath, JSON.stringify(report));
  await writeFile(reviewPath, JSON.stringify({ 'sample-rainbow-bear': review }));
  await run('node', [script, reportPath, reviewPath, outPath, '--pack', join(dir, 'packs')]);
  const out = JSON.parse(await readFile(outPath, 'utf8'));
  return { game: out.games[0], dir };
}

test('two blind passes merge; a >15 spread is contested and median-scored with a third pass', async () => {
  const review = {
    judge: { model: 'judge-x', version: '1' },
    passes: [pass(80, 50), pass(82, 90), pass(81, 70)],
  };
  const { game } = await runScorer(review);
  const plot = game.quality.dimensions.find(d => d.id === 'plot');
  assert.equal(plot.score, 70); // median of 50/90/70
  assert.equal(plot.contested, true);
  const visual = game.quality.dimensions.find(d => d.id === 'visual_design');
  assert.equal(visual.score, 81);
  assert.equal(visual.contested, undefined);
  assert.deepEqual(game.quality.provenance.judge, { model: 'judge-x', version: '1' });
  assert.equal(game.quality.provenance.passes, 3);
  assert.ok(game.quality.provenance.packHash);
  // Graded, not saturated: jank 2% + p95 21ms + first input 900ms
  // pull performance/input below 100 even with every check passing.
  assert.equal(game.quality.subscores.measured.score, 97.9);
  assert.equal(game.quality.schemaVersion, '1.3');
});

test('measured-evidence suggester covers weak objective metrics; creative gaps stay honestly missing', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'dq-score-'));
  const weakReport = {
    games: [{
      gameId: 'sample-rainbow-bear', title: "Bobo's Rainbow", checks,
      probe: { status: 'timeout', census: 'x', errors: [], warnCount: 0, jankRate: 0.15, p95FrameMs: 50, deadInputRate: 0.3, firstInputMs: 2900, firstRewardMs: null, events: [] },
    }],
  };
  const reportPath = join(dir, 'report.json');
  const outPath = join(dir, 'out.json');
  const mdPath = join(dir, 'suggestions.md');
  await writeFile(reportPath, JSON.stringify(weakReport));
  await run('node', [script, reportPath, '-', outPath, '--suggestions-md', mdPath]);
  const out = JSON.parse(await readFile(outPath, 'utf8'));
  const q = out.games[0].quality;
  const measured = q.improvementSuggestions.filter(s => s.source === 'measured');
  assert.ok(measured.some(s => s.metricId === 'performance'), 'performance gets a measured suggestion');
  assert.ok(measured.some(s => s.metricId === 'input_responsiveness'), 'input gets a measured suggestion');
  const missingIds = q.missingSuggestions.map(m => m.metricId);
  assert.ok(!missingIds.includes('performance') && !missingIds.includes('input_responsiveness'));
  assert.ok(missingIds.includes('visual_design'), 'unreviewed creative dimensions remain honestly missing');
  const md = await readFile(mdPath, 'utf8');
  assert.ok(md.includes('source: measured'));
});

test('evidence pack judge view carries observations, never check statuses or scores', async () => {
  const { dir } = await runScorer({ judge: { model: 'judge-x', version: '1' }, passes: [pass(80)] });
  const view = JSON.parse(await readFile(join(dir, 'packs', 'sample-rainbow-bear', 'judge-view.json'), 'utf8'));
  assert.equal(view.gameId, 'sample-rainbow-bear');
  assert.equal(view.observations.status, 'won');
  const text = JSON.stringify(view);
  assert.ok(!text.includes('boot_clean'), 'judge view must not leak check ids/statuses');
  assert.ok(!text.includes('"score"'), 'judge view must not leak scores');
  await access(join(dir, 'packs', 'sample-rainbow-bear', 'pack.json'));
});
