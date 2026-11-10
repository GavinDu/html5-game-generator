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
  const r = evaluatePenguinPacing({...base, progressAtIntroClear: 11, winAtSec: 8});
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
