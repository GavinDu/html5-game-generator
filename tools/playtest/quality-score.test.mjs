import test from 'node:test';
import assert from 'node:assert/strict';
import { METRICS, scoreGame } from '../../packages/game-quality-score/index.mjs';

const passing = [
  { checkId: 'playtest.boot_clean', status: 'pass', evidence: '0 runtime errors' },
  { checkId: 'playtest.render_nonblank', status: 'pass', evidence: 'visible entities' },
  { checkId: 'simulation.no_softlock', status: 'pass', evidence: 'state advanced' },
  { checkId: 'hook.time_to_input', status: 'pass', evidence: 'input prompt at 200ms' },
  { checkId: 'hook.first_reward', status: 'pass', evidence: 'reward at 2s' },
  { checkId: 'budgets.cold_start', status: 'pass', evidence: 'startup < 3s', actual: { coldStartMs: 1200 } },
  { checkId: 'simulation.win_reachable', status: 'pass', evidence: 'bot won' },
];

const review = {
  metrics: METRICS.filter((m) => m.source === 'ai').map((m) => ({
    id: m.id, score: 85, rationale: 'Clear and coherent for the intended audience.',
    evidence: 'Observed in the game scene and its declared design.',
  })),
};

test('weights sum to 100 and all metric IDs are unique', () => {
  assert.equal(METRICS.reduce((sum, m) => sum + m.weight, 0), 100);
  assert.equal(new Set(METRICS.map((m) => m.id)).size, METRICS.length);
});

test('withholds overall score until every dimension has evidence', () => {
  const result = scoreGame(passing);
  assert.equal(result.overallScore, null);
  assert.equal(result.deliveryReady, true);
  assert.equal(result.deliveryGate, 'pass');
  assert.equal(result.partialScore, 100);
  assert.equal(result.coveragePercent, 48);
  assert.ok(result.missingMetrics.includes('visual_design'));
});

test('combines objective evidence and AI review into a complete score', () => {
  const result = scoreGame(passing, review);
  assert.equal(result.overallScore, 92.2);
  assert.equal(result.overallScore, result.partialScore);
  assert.equal(result.grade, 'excellent');
  assert.equal(result.coveragePercent, 100);
  assert.equal(result.deliveryReady, true);
});

test('hard-gate failure blocks delivery even with excellent AI scores', () => {
  const failed = passing.map((c) => c.checkId === 'playtest.render_nonblank' ? { ...c, status: 'fail' } : c);
  const result = scoreGame(failed, review);
  assert.equal(result.deliveryReady, false);
  assert.equal(result.deliveryGate, 'fail');
  // Schema 1.3 (metric catalog fusion): the failed gate zeroes
  // playability, and any dimension under 35 caps the composite at 65 —
  // the uncapped blend (72.2) is preserved for inspection.
  assert.equal(result.uncappedOverallScore, 72.2);
  assert.equal(result.overallScore, 65);
  assert.ok(result.caps.some((c) => c.rule === 'weak_dimension_cap'));
});

test('AI cannot override objective metrics and unsupported scores are ignored', () => {
  const spoof = { metrics: [
    { id: 'reliability', score: 100, rationale: 'great', evidence: 'trust me' },
    { id: 'visual_design', score: 1000, rationale: 'great', evidence: 'invalid range' },
  ] };
  const result = scoreGame(passing, spoof);
  assert.equal(result.dimensions.find((d) => d.id === 'reliability').score, 100);
  assert.equal(result.dimensions.find((d) => d.id === 'visual_design').score, null);
});
