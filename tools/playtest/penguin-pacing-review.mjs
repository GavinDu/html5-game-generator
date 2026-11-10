// Advisory creative-pacing review from actual runtime snapshots.
// This is NOT a substitute for a child/family playtest or a release gate.
export function evaluatePenguinPacing({ sessionTargetSec, introDeadlineSec, winAtSec, progressAtIntroClear, introSession, milestoneSeconds }) {
  const valid = n => Number.isFinite(n) && n >= 0;
  if (!valid(sessionTargetSec) || sessionTargetSec === 0 ||
      !valid(introDeadlineSec) || !valid(winAtSec) ||
      !Array.isArray(milestoneSeconds) ||
      milestoneSeconds.some(m => !valid(m.hearts) || !valid(m.runtimeSeconds))) {
    return { status: 'unmeasured', needsCreativeReview: true, reasons: ['invalid-or-missing-timing'] };
  }
  // A win snapshot does not prove when intermediate milestones occurred.
  // Do not accept a fabricated sequence back-filled at the final win time,
  // duplicate milestones, or out-of-order / post-win timestamps as complete
  // creative evidence. This advisory never changes the game's hard PASS gate.
  const requiredHearts = [2, 4, 6, 8, 10, 12];
  const byHearts = new Map();
  let inconsistent = false;
  for (const milestone of milestoneSeconds) {
    if (!requiredHearts.includes(milestone.hearts) || byHearts.has(milestone.hearts) ||
        milestone.runtimeSeconds > winAtSec) {
      inconsistent = true;
    }
    byHearts.set(milestone.hearts, milestone.runtimeSeconds);
  }
  if (!requiredHearts.every(h => byHearts.has(h)) || inconsistent ||
      requiredHearts.some((h, i) => i > 0 && byHearts.get(h) < byHearts.get(requiredHearts[i - 1]))) {
    return {
      status: 'unmeasured', needsCreativeReview: true,
      reasons: ['missing-or-inconsistent-milestone-observation'],
      milestoneSeconds,
      missingMilestones: requiredHearts.filter(h => !byHearts.has(h))
    };
  }
  const completionFraction = winAtSec / sessionTargetSec;
  const rapidGuidedCompletion = completionFraction < 0.3;
  // Only treat overlay occlusion as observed if the screenshot is
  // captured in the SAME guided play session as the win trace.
  const nearVictoryUnderIntro = introSession === 'guided-play' &&
    valid(progressAtIntroClear) && progressAtIntroClear >= 10;
  const reasons = [];
  if (rapidGuidedCompletion) reasons.push('guided-win-much-faster-than-session-target');
  if (nearVictoryUnderIntro) reasons.push('intro-cleared-near-completion');
  return {
    status: reasons.length ? 'review-pacing' : 'no-advisory',
    needsCreativeReview: reasons.length > 0,
    reasons, completionFraction, sessionTargetSec, introDeadlineSec, winAtSec,
    progressAtIntroClear: valid(progressAtIntroClear) ? progressAtIntroClear : null,
    introSession, milestoneSeconds
  };
}
