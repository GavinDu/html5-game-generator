// Standalone, evidence-backed game quality scoring.
// Objective scores come from browser evidence; creative scores require a cited AI review.
export const METRICS = [
  { id: 'playability', label: 'Minimum playability', weight: 20, source: 'objective', description: 'Boot, visible scene, responsive controls, progression, and no softlock.' },
  { id: 'reliability', label: 'Runtime reliability', weight: 12, source: 'objective', description: 'No uncaught exceptions, failed assets, or broken runtime states.' },
  { id: 'input_responsiveness', label: 'Input responsiveness', weight: 8, source: 'objective', description: 'Controls respond quickly and produce meaningful feedback.' },
  { id: 'performance', label: 'Performance', weight: 8, source: 'objective', description: 'Startup, frame stability, input latency, and resource budgets.' },
  { id: 'visual_design', label: 'Visual beauty and cohesion', weight: 8, source: 'ai', description: 'Composition, color, contrast, hierarchy, consistency, and polish.' },
  { id: 'character_design', label: 'Character design and readability', weight: 7, source: 'ai', description: 'Silhouette, personality, readability, and fit to the premise.' },
  { id: 'animation', label: 'Animation and game feel', weight: 7, source: 'ai', description: 'Motion, timing, transitions, impact, and perceived responsiveness.' },
  { id: 'plot', label: 'Plot and emotional arc', weight: 7, source: 'ai', description: 'Premise, progression, stakes, payoff, and emotional resonance.' },
  { id: 'strategy', label: 'Strategy and meaningful choices', weight: 8, source: 'ai', description: 'Player agency, trade-offs, skill expression, and meaningful decisions.' },
  { id: 'level_design', label: 'Level and progression design', weight: 5, source: 'ai', description: 'Pacing, spatial readability, variety, difficulty curve, and achievable goals.' },
  { id: 'audio_feedback', label: 'Audio and feedback', weight: 4, source: 'ai', description: 'Useful sound, feedback clarity, mix, and mute behavior.' },
  { id: 'originality', label: 'Originality and coherence', weight: 4, source: 'ai', description: 'Distinctive idea and mechanics that reinforce the theme.' },
  { id: 'accessibility', label: 'Usability and accessibility', weight: 2, source: 'ai', description: 'Readable text, clear controls, reasonable difficulty, and inclusive interaction.' },
];
export const DEFAULT_RULES = {
  schemaVersion: '1.0',
  profile: 'default',
  thresholds: { minimumOverallScore: 80, minimumMetricScore: 70, minimumMeasuredScore: 60, requireCompleteCoverage: true },
  hardGates: ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock'],
  weights: Object.fromEntries(METRICS.map(m => [m.id, m.weight])),
  reviewFocus: [],
  gameplayContract: {},
};
const OBJECTIVE_IDS = new Set(METRICS.filter(m => m.source === 'objective').map(m => m.id));
const clamp = (n, lo = 0, hi = 100) => Math.max(lo, Math.min(hi, n));
const round = n => Math.round(n * 10) / 10;
const statusOf = (checks, id) => checks.find(c => c.checkId === id)?.status ?? null;
const scoreForStatus = status => status === 'pass' ? 100 : status === 'fail' ? 0 : null;
function measured(checks, id) { return scoreForStatus(statusOf(checks, id)); }
function responseScore(checks) {
  const values = ['hook.time_to_input', 'hook.first_reward'].map(id => scoreForStatus(statusOf(checks, id))).filter(v => v !== null);
  return values.length ? values.reduce((a, b) => a + b, 0) / values.length : null;
}
function performanceScore(checks) {
  const c = checks.find(x => x.checkId === 'budgets.cold_start');
  if (!c) return null;
  if (c.status === 'pass') return 100;
  if (c.status !== 'fail') return null;
  const ms = c.actual?.coldStartMs;
  return Number.isFinite(ms) ? round(clamp(100 * (10000 - ms) / 7000)) : 0;
}
function playabilityScore(checks) {
  // Graded (schema 1.2): the gate trio still zeroes the metric on a
  // hard failure and leaves it unscored until all three pass; beyond
  // the gates, measured play evidence deducts — an unobserved win,
  // failed shell conformance, failed determinism/replay, or no first
  // reward all lower the score instead of saturating at 100.
  const statuses = ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock'].map(id => statusOf(checks, id));
  if (statuses.includes('fail')) return 0;
  if (!statuses.every(s => s === 'pass')) return null;
  let s = 100;
  if (statusOf(checks, 'simulation.win_reachable') !== 'pass') s -= 15;
  for (const id of ['shell.start_card', 'shell.result_card', 'shell.restart_in_place']) {
    if (statusOf(checks, id) === 'fail') s -= 10;
  }
  if (statusOf(checks, 'simulation.determinism') === 'fail') s -= 10;
  if (statusOf(checks, 'leaderboard.replay_verified') === 'fail') s -= 10;
  const fr = statusOf(checks, 'hook.first_reward');
  if (fr !== 'pass') s -= 5;
  return Math.max(0, s);
}
// Anchored linear curve: value at/below `good` scores 100, at/above
// `bad` scores 0. Returns null for non-finite input.
function curve(value, good, bad) {
  if (!Number.isFinite(value)) return null;
  return round(clamp(100 * (bad - value) / (bad - good)));
}
function reliabilityScore(checks, m) {
  if (m.errorCount === undefined && m.warnClasses === undefined) return measured(checks, 'playtest.boot_clean');
  if (statusOf(checks, 'playtest.boot_clean') === 'fail') return 0;
  return clamp(100 - 25 * (m.errorCount || 0) - 10 * (m.warnClasses || 0));
}
function inputScore(checks, m) {
  const parts = [];
  const fi = curve(m.firstInputMs, 300, 3000);
  if (fi !== null) parts.push([fi, 0.4]);
  if (Number.isFinite(m.firstRewardMs)) {
    const target = (Number.isFinite(m.firstRewardTargetSec) && m.firstRewardTargetSec > 0 ? m.firstRewardTargetSec : 5) * 1000;
    const ratio = m.firstRewardMs / target;
    parts.push([ratio <= 1 ? 100 : round(clamp(100 - (ratio - 1) * 60)), 0.4]);
  }
  if (Number.isFinite(m.deadInputRate)) {
    parts.push([round(clamp(100 * (0.25 - m.deadInputRate) / 0.25)), 0.2]);
  }
  if (!parts.length) return responseScore(checks);
  const w = parts.reduce((a, [, x]) => a + x, 0);
  return round(parts.reduce((a, [v, x]) => a + v * x, 0) / w);
}
function performanceGraded(checks, m) {
  const parts = [];
  const cold = performanceScore(checks);
  if (cold !== null) parts.push(cold);
  if (Number.isFinite(m.jankRate)) parts.push(round(clamp(100 * (0.15 - m.jankRate) / 0.14)));
  const p = curve(m.p95FrameMs, 20, 50);
  if (p !== null) parts.push(p);
  if (!parts.length) return null;
  return round(parts.reduce((a, b) => a + b, 0) / parts.length);
}
function grade(score) {
  if (score >= 90) return 'excellent';
  if (score >= 80) return 'strong';
  if (score >= 70) return 'good';
  if (score >= 60) return 'needs_polish';
  return 'weak';
}
function normalizeRules(rules = {}) {
  const result = {
    ...DEFAULT_RULES, ...rules,
    thresholds: { ...DEFAULT_RULES.thresholds, ...(rules.thresholds || {}) },
    hardGates: rules.hardGates || DEFAULT_RULES.hardGates,
    weights: { ...DEFAULT_RULES.weights, ...(rules.weights || {}) },
  };
  const total = METRICS.reduce((sum, m) => sum + Number(result.weights[m.id] || 0), 0);
  if (METRICS.some(m => !Number.isFinite(result.weights[m.id]) || result.weights[m.id] < 0) || Math.abs(total - 100) > 0.01) {
    throw new Error('Quality rules must define non-negative weights totaling 100 across all metrics.');
  }
  return result;
}

/**
 * scoreGame(checks, aiReview, rules)
 * aiReview.metrics: {id, score: 0..100, rationale, evidence}[]
 * aiReview.improvementSuggestions: {metricId, priority, problem, suggestion, evidence?}[]
 */
export function scoreGame(checks = [], aiReview = null, suppliedRules = {}, measurements = {}) {
  const rules = normalizeRules(suppliedRules);
  const aiMetrics = new Map((aiReview?.metrics || []).map(m => [m.id, m]));
  const objective = {
    playability: playabilityScore(checks),
    reliability: reliabilityScore(checks, measurements),
    input_responsiveness: inputScore(checks, measurements),
    performance: performanceGraded(checks, measurements),
  };
  const dimensions = METRICS.map(metric => {
    let score = null, evidence = '', rationale = '';
    if (OBJECTIVE_IDS.has(metric.id)) {
      score = objective[metric.id];
      const related = {
        playability: ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock', 'simulation.win_reachable', 'shell.start_card', 'shell.result_card', 'shell.restart_in_place', 'simulation.determinism', 'leaderboard.replay_verified', 'hook.first_reward'],
        reliability: ['playtest.boot_clean'],
        input_responsiveness: ['hook.time_to_input', 'hook.first_reward'],
        performance: ['budgets.cold_start'],
      }[metric.id];
      evidence = related.map(id => checks.find(c => c.checkId === id)).filter(Boolean)
        .map(c => c.checkId + '=' + c.status + (c.evidence ? ' (' + c.evidence + ')' : '')).join('; ');
      const meas = {
        playability: '',
        reliability: measurements.errorCount !== undefined ? `; errors=${measurements.errorCount}, warnClasses=${measurements.warnClasses || 0}` : '',
        input_responsiveness: [measurements.firstInputMs, measurements.firstRewardMs, measurements.deadInputRate]
          .some(v => v !== undefined && v !== null)
          ? `; firstInputMs=${measurements.firstInputMs ?? 'n/a'}, firstRewardMs=${measurements.firstRewardMs ?? 'n/a'}, deadInputRate=${measurements.deadInputRate ?? 'n/a'}` : '',
        performance: measurements.jankRate !== undefined || measurements.p95FrameMs !== undefined
          ? `; jankRate=${measurements.jankRate ?? 'n/a'}, p95FrameMs=${measurements.p95FrameMs ?? 'n/a'}` : '',
      }[metric.id];
      evidence += meas;
      rationale = score === null ? 'Insufficient browser evidence; unscored.' : score === 100 ? 'Measured evidence at full marks.' : score === 0 ? 'A required check failed.' : 'Graded from measured browser evidence (see evidence).';
    } else {
      const review = aiMetrics.get(metric.id);
      if (review && Number.isFinite(review.score) && review.score >= 0 && review.score <= 100 &&
          typeof review.rationale === 'string' && review.rationale.trim() &&
          typeof review.evidence === 'string' && review.evidence.trim()) {
        score = round(review.score); evidence = review.evidence.trim(); rationale = review.rationale.trim();
      }
    }
    const dim = { id: metric.id, label: metric.label, weight: rules.weights[metric.id], score, evidence, rationale };
    const review = OBJECTIVE_IDS.has(metric.id) ? null : aiMetrics.get(metric.id);
    if (review && review.contested) dim.contested = true;
    return dim;
  });
  const available = dimensions.filter(d => d.score !== null);
  const weightAvailable = available.reduce((sum, d) => sum + d.weight, 0);
  const weighted = available.reduce((sum, d) => sum + d.score * d.weight, 0);
  const coveragePercent = round(weightAvailable);
  const partialScore = weightAvailable ? round(weighted / weightAvailable) : null;
  const missingMetrics = dimensions.filter(d => d.score === null).map(d => d.id);
  const gates = rules.hardGates.map(id => ({ id, status: statusOf(checks, id) }));
  const deliveryGate = gates.every(g => g.status === 'pass') ? 'pass' : gates.some(g => g.status === 'fail') ? 'fail' : 'inconclusive';
  const floor = rules.thresholds.minimumMetricScore;
  const belowFloor = dimensions.filter(d => d.score !== null && d.score < floor).map(d => ({ metricId: d.id, score: d.score, minimum: floor }));
  const suggestionRequired = dimensions.filter(d => d.score === null || d.score < floor).map(d => ({ metricId: d.id, score: d.score }));
  const missingSuggestions = suggestionRequired.filter(item => !(aiReview?.improvementSuggestions || []).some(s => s.metricId === item.metricId && typeof s.suggestion === 'string' && s.suggestion.trim()));
  const improvementSuggestions = (aiReview?.improvementSuggestions || []).filter(s =>
    s && typeof s.metricId === 'string' && typeof s.suggestion === 'string' && s.suggestion.trim());
  const allMetricsMeetFloor = dimensions.every(d => d.score !== null && d.score >= floor);
  const completeCoverage = missingMetrics.length === 0;
  const overallTargetMet = partialScore !== null && partialScore >= rules.thresholds.minimumOverallScore;
  // Subscores (schema 1.2): the measured (objective) and creative
  // (judged) halves are always reported separately — they answer
  // different questions and drive different actions. A failed hard
  // gate caps the measured subscore at 40: a game that does not boot
  // cleanly cannot buy its way out with fast load times.
  const subscore = (ids) => {
    const dims = dimensions.filter(d => ids.has(d.id) && d.score !== null);
    const w = dims.reduce((sum, d) => sum + d.weight, 0);
    if (!w) return null;
    return round(dims.reduce((sum, d) => sum + d.score * d.weight, 0) / w);
  };
  let measuredScore = subscore(OBJECTIVE_IDS);
  if (measuredScore !== null && deliveryGate === 'fail') measuredScore = Math.min(measuredScore, 40);
  const creativeScore = subscore(new Set(METRICS.filter(m => m.source === 'ai').map(m => m.id)));
  const measuredFloor = Number.isFinite(rules.thresholds.minimumMeasuredScore) ? rules.thresholds.minimumMeasuredScore : 0;
  const measuredTargetMet = measuredScore !== null && measuredScore >= measuredFloor;
  const targetMet = overallTargetMet && allMetricsMeetFloor && measuredTargetMet &&
    (!rules.thresholds.requireCompleteCoverage || completeCoverage) && deliveryGate === 'pass';
  const reviewRequests = dimensions.filter(d => d.score === null || d.score < floor).map(d => ({
    metricId: d.id, currentScore: d.score, minimumScore: floor,
    reason: d.score === null ? 'missing_evidence_or_review' : 'below_metric_floor',
    requiredOutput: 'Provide evidence-backed diagnosis and a concrete, prioritized improvement suggestion.',
  }));
  return {
    schemaVersion: '1.2',
    profile: rules.profile,
    thresholds: rules.thresholds,
    subscores: {
      measured: { score: measuredScore, weight: METRICS.filter(m => m.source === 'objective').reduce((s, m) => s + rules.weights[m.id], 0) },
      creative: { score: creativeScore, weight: METRICS.filter(m => m.source === 'ai').reduce((s, m) => s + rules.weights[m.id], 0) },
    },
    overallScore: completeCoverage ? partialScore : null,
    partialScore,
    coveragePercent,
    grade: completeCoverage ? grade(partialScore) : 'incomplete',
    deliveryGate,
    deliveryReady: deliveryGate === 'pass',
    hardGates: gates,
    missingMetrics,
    dimensions,
    weightsTotal: METRICS.reduce((sum, m) => sum + rules.weights[m.id], 0),
    target: {
      minimumOverallScore: rules.thresholds.minimumOverallScore,
      minimumMetricScore: floor,
      minimumMeasuredScore: measuredFloor,
      measuredScore,
      measuredTargetMet,
      overallTargetMet,
      allMetricsMeetFloor,
      completeCoverage,
      targetMet,
    },
    belowFloor,
    improvementSuggestions,
    missingSuggestions,
    reviewRequests,
    improvementRequired: !targetMet,
  };
}
