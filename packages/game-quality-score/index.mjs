// Evidence-backed game quality scoring for the browser playtest kit.
// Objective runtime metrics come from Playwright checks; subjective metrics
// must come from an AI review with concrete evidence. Missing evidence stays
// null and is never silently converted into a passing score.

export const METRICS = [
  { id: 'playability', label: 'Minimum playability', weight: 20, source: 'objective', description: 'Game boots, renders, accepts input, progresses, and does not softlock.' },
  { id: 'reliability', label: 'Runtime reliability', weight: 12, source: 'objective', description: 'No uncaught exceptions, failed assets, or broken runtime states.' },
  { id: 'input_responsiveness', label: 'Input responsiveness', weight: 8, source: 'objective', description: 'Controls become usable quickly and player input causes observable response.' },
  { id: 'performance', label: 'Performance', weight: 8, source: 'objective', description: 'Startup latency, frame stability, asset cost, memory, and input latency.' },
  { id: 'visual_design', label: 'Visual beauty and cohesion', weight: 8, source: 'ai', description: 'Composition, color, contrast, hierarchy, consistency, and visual polish.' },
  { id: 'character_design', label: 'Character design and readability', weight: 7, source: 'ai', description: 'Distinct silhouette, personality, animation readability, and fit to the premise.' },
  { id: 'animation', label: 'Animation and game feel', weight: 7, source: 'ai', description: 'Motion quality, timing, transitions, feedback, and perceived responsiveness.' },
  { id: 'plot', label: 'Plot and emotional arc', weight: 7, source: 'ai', description: 'Clear premise, coherent progression, stakes, payoff, and emotional resonance.' },
  { id: 'strategy', label: 'Strategy and meaningful choices', weight: 8, source: 'ai', description: 'Player agency, understandable trade-offs, skill expression, and nontrivial decisions.' },
  { id: 'level_design', label: 'Level and progression design', weight: 5, source: 'ai', description: 'Pacing, spatial readability, variety, difficulty curve, and achievable goals.' },
  { id: 'audio_feedback', label: 'Audio and feedback', weight: 4, source: 'ai', description: 'Useful sound/music, event feedback, mix, and appropriate mute behavior.' },
  { id: 'originality', label: 'Originality and coherence', weight: 4, source: 'ai', description: 'Distinctive idea and mechanics that reinforce rather than contradict the theme.' },
  { id: 'accessibility', label: 'Usability and accessibility', weight: 2, source: 'ai', description: 'Readable text, clear controls, reasonable difficulty, and inclusive interaction.' },
];

const OBJECTIVE_IDS = new Set(METRICS.filter((m) => m.source === 'objective').map((m) => m.id));
const clamp = (n, lo = 0, hi = 100) => Math.max(lo, Math.min(hi, n));
const round = (n) => Math.round(n * 10) / 10;

function statusOf(checks, id) {
  return checks.find((c) => c.checkId === id)?.status ?? null;
}
function scoreForStatus(status) {
  return status === 'pass' ? 100 : status === 'fail' ? 0 : null;
}
function measured(checks, id) {
  return scoreForStatus(statusOf(checks, id));
}
function responseScore(checks) {
  const timing = checks.find((c) => c.checkId === 'hook.time_to_input');
  const reward = checks.find((c) => c.checkId === 'hook.first_reward');
  const values = [];
  if (timing?.status === 'pass') values.push(100);
  else if (timing?.status === 'fail') values.push(0);
  if (reward?.status === 'pass') values.push(100);
  else if (reward?.status === 'fail') values.push(0);
  return values.length ? values.reduce((a, b) => a + b, 0) / values.length : null;
}
function performanceScore(checks) {
  const check = checks.find((c) => c.checkId === 'budgets.cold_start');
  if (!check) return null;
  if (check.status === 'pass') return 100;
  if (check.status === 'fail') {
    const ms = check.actual?.coldStartMs;
    if (Number.isFinite(ms)) return round(clamp(100 * (10000 - ms) / 7000));
    return 0;
  }
  return null;
}
function playabilityScore(checks) {
  const required = ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock'];
  const statuses = required.map((id) => statusOf(checks, id));
  if (statuses.includes('fail')) return 0;
  if (statuses.every((s) => s === 'pass')) return 100;
  return null;
}

/**
 * Combine Playwright evidence with an optional AI review.
 * aiReview.metrics is an array of { id, score: 0..100, rationale, evidence }.
 * Objective metrics cannot be overridden by the AI. Unknown or evidence-free
 * subjective scores are ignored. Overall score is withheld until all metrics
 * have evidence; a partial weighted score and coverage are still returned.
 */
export function scoreGame(checks = [], aiReview = null) {
  const aiMetrics = new Map((aiReview?.metrics || []).map((m) => [m.id, m]));
  const objective = {
    playability: playabilityScore(checks),
    reliability: measured(checks, 'playtest.boot_clean'),
    input_responsiveness: responseScore(checks),
    performance: performanceScore(checks),
  };
  const dimensions = METRICS.map((metric) => {
    let score = null;
    let evidence = '';
    let rationale = '';
    if (OBJECTIVE_IDS.has(metric.id)) {
      score = objective[metric.id];
      const related = {
        playability: ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock'],
        reliability: ['playtest.boot_clean'],
        input_responsiveness: ['hook.time_to_input', 'hook.first_reward'],
        performance: ['budgets.cold_start'],
      }[metric.id];
      evidence = related.map((id) => checks.find((c) => c.checkId === id))
        .filter(Boolean).map((c) => `${c.checkId}=${c.status}${c.evidence ? ` (${c.evidence})` : ''}`).join('; ');
      rationale = score === null ? 'Insufficient browser evidence; this dimension is unscored.' :
        score === 100 ? 'Measured checks passed.' : score === 0 ? 'A required check failed.' : 'Score derived from measured browser evidence.';
    } else {
      const review = aiMetrics.get(metric.id);
      if (review && Number.isFinite(review.score) && review.score >= 0 && review.score <= 100 &&
          typeof review.rationale === 'string' && review.rationale.trim() &&
          typeof review.evidence === 'string' && review.evidence.trim()) {
        score = round(review.score);
        evidence = review.evidence.trim();
        rationale = review.rationale.trim();
      }
    }
    return { id: metric.id, label: metric.label, weight: metric.weight, score, evidence, rationale };
  });
  const available = dimensions.filter((d) => d.score !== null);
  const weightAvailable = available.reduce((sum, d) => sum + d.weight, 0);
  const weighted = available.reduce((sum, d) => sum + d.score * d.weight, 0);
  const coverage = round(weightAvailable);
  const partialScore = weightAvailable ? round(weighted / weightAvailable) : null;
  const missing = dimensions.filter((d) => d.score === null).map((d) => d.id);
  const gates = ['playtest.boot_clean', 'playtest.render_nonblank', 'simulation.no_softlock'];
  const gateStatuses = gates.map((id) => ({ id, status: statusOf(checks, id) }));
  const hardGate = gateStatuses.every((g) => g.status === 'pass');
  return {
    schemaVersion: '1.0',
    overallScore: missing.length === 0 ? partialScore : null,
    partialScore,
    coveragePercent: coverage,
    grade: missing.length === 0 ? grade(partialScore) : 'incomplete',
    deliveryGate: hardGate ? 'pass' : gateStatuses.some((g) => g.status === 'fail') ? 'fail' : 'inconclusive',
    deliveryReady: hardGate,
    hardGates: gateStatuses,
    missingMetrics: missing,
    dimensions,
    weightsTotal: METRICS.reduce((sum, m) => sum + m.weight, 0),
  };
}
function grade(score) {
  if (score >= 90) return 'excellent';
  if (score >= 80) return 'strong';
  if (score >= 70) return 'good';
  if (score >= 60) return 'needs_polish';
  return 'weak';
}
