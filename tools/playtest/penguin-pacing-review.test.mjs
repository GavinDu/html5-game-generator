import test from 'node:test';
import assert from 'node:assert/strict';
import { evaluatePenguinPacing } from './penguin-pacing-review.mjs';
const base = {
  sessionTargetSec: 55, introDeadlineSec: 7, winAtSec: 35,
  progressAtIntroClear: 2, introSession: 'guided-play',
  milestoneSeconds: [
    { hearts: 2, runtimeSeconds: 6 }, { hearts: 4, runtimeSeconds: 12 },
    { hearts: 6, runtimeSeconds: 18 }, { hearts: 8, runtimeSeconds: 24 },
    { hearts: 10, runtimeSeconds: 30 }, { hearts: 12, runtimeSeconds: 35 }
  ],
};
test('normal guided completion stays advisory-free', () => {
  const result = evaluatePenguinPacing(base);
  assert.equal(result.status, 'no-advisory');
  assert.equal(result.needsCreativeReview, false);
});
test('near-complete heart objective during the seven-second intro is flagged', () => {
  const rapidMilestones = [2, 4, 6, 8, 10, 12]
    .map((hearts, i) => ({ hearts, runtimeSeconds: (i + 1) * 1.3 }));
  const r = evaluatePenguinPacing({...base, progressAtIntroClear: 11, winAtSec: 8,
    milestoneSeconds: rapidMilestones });
  assert.deepEqual(r.reasons, ['guided-win-much-faster-than-session-target','intro-cleared-near-completion']);
  assert.equal(r.needsCreativeReview, true);
});
test('independent intro screenshot must not be mistaken for same-session milestone', () => {
  const r = evaluatePenguinPacing({...base, introSession: 'independent-real-browser', progressAtIntroClear: 11});
  assert.equal(r.needsCreativeReview, false);
});
test('invalid timing is unmeasured, never silently approved', () => {
  assert.equal(evaluatePenguinPacing({...base, winAtSec: Number.NaN}).status, 'unmeasured');
  assert.equal(evaluatePenguinPacing({...base, milestoneSeconds: undefined}).needsCreativeReview, true);
});

test('an unobserved intermediate milestone remains unmeasured instead of inheriting win time', () => {
  const observed = base.milestoneSeconds.filter(m => m.hearts !== 8);
  const r = evaluatePenguinPacing({...base, milestoneSeconds: observed});
  assert.equal(r.status, 'unmeasured');
  assert.equal(r.needsCreativeReview, true);
  assert.deepEqual(r.missingMilestones, [8]);
  assert.deepEqual(r.milestoneSeconds, observed);
});
test('duplicate, reversed, or after-win stage timings never become creative PASS', () => {
  const duplicate = [...base.milestoneSeconds, { hearts: 12, runtimeSeconds: 35 }];
  assert.equal(evaluatePenguinPacing({...base, milestoneSeconds: duplicate}).status, 'unmeasured');
  const backwards = base.milestoneSeconds.map(m => m.hearts === 8 ? {...m, runtimeSeconds: 5} : m);
  assert.equal(evaluatePenguinPacing({...base, milestoneSeconds: backwards}).status, 'unmeasured');
  const afterWin = base.milestoneSeconds.map(m => m.hearts === 12 ? {...m, runtimeSeconds: 40} : m);
  assert.equal(evaluatePenguinPacing({...base, milestoneSeconds: afterWin}).status, 'unmeasured');
});

test('verified v6 first reward prevents stale seven-second occlusion advisory', () => {
  const rapidMilestones = [2, 4, 6, 8, 10, 12]
    .map((hearts, i) => ({ hearts, runtimeSeconds: (i + 1) * 1.2 }));
  const earlyRevealEvidence = {
    session: 'guided-play', runtimeSeconds: 4.8, hearts: 4,
    screenshot: 'first-reward-reveal.png'
  };
  const r = evaluatePenguinPacing({ ...base, progressAtIntroClear: 11,
    winAtSec: 7.6, milestoneSeconds: rapidMilestones, earlyRevealEvidence });
  assert.deepEqual(r.reasons, ['guided-win-much-faster-than-session-target']);
  assert.equal(r.earlyRevealVerified, true);
  assert.equal(r.earlyRevealEvidence.runtimeSeconds, 4.8);
  assert.equal(r.needsCreativeReview, true,
    'fast scripted completion still requires an independent human pacing review');
});
test('independent or impossible early reveal cannot suppress real occlusion', () => {
  const rapidMilestones = [2, 4, 6, 8, 10, 12]
    .map((hearts, i) => ({ hearts, runtimeSeconds: (i + 1) * 1.2 }));
  const item = { ...base, progressAtIntroClear: 11, winAtSec: 7.6,
    milestoneSeconds: rapidMilestones };
  for (const earlyRevealEvidence of [
    { session: 'independent-real-browser', runtimeSeconds: 4.8, hearts: 4 },
    { session: 'guided-play', runtimeSeconds: 8.1, hearts: 4 },
    { session: 'guided-play', runtimeSeconds: 4.8, hearts: 0 },
    { session: 'guided-play', runtimeSeconds: 4.8, hearts: 13 }
  ]) {
    const r = evaluatePenguinPacing({ ...item, earlyRevealEvidence });
    assert.equal(r.earlyRevealVerified, false);
    assert.ok(r.reasons.includes('intro-cleared-near-completion'));
  }
});
