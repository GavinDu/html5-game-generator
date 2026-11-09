import test from 'node:test';
import assert from 'node:assert/strict';
import { validatePenguinIntroEvidence } from './preview-evidence.mjs';

const valid = {
  passed: true, won: true, introClearedAfterSevenSeconds: true,
  compactHintPersists: true, screenshots: ['intro-cleared', 'win'],
  introEvidence: { session: 'guided-play', runtimeSeconds: 8.25, objectiveProgress: 6 },
};

test('verified in-run and independent real-browser screenshot captures preserve provenance', () => {
  assert.deepEqual(validatePenguinIntroEvidence(valid), valid.introEvidence);
  const independent = {...valid, introEvidence: {
    session: 'independent-real-browser', runtimeSeconds: 7.35, objectiveProgress: 0,
  }};
  assert.deepEqual(validatePenguinIntroEvidence(independent), independent.introEvidence);
});

test('missing or guessed capture session, runtime or progress never becomes approved poster evidence', () => {
  for (const introEvidence of [
    undefined, {},
    { session: 'guided-play', runtimeSeconds: 0, objectiveProgress: 6 },
    { session: 'pretend', runtimeSeconds: 8.2, objectiveProgress: 6 },
    { session: 'guided-play', runtimeSeconds: 8.2, objectiveProgress: 12 },
    { session: 'independent-real-browser', runtimeSeconds: 8.2, objectiveProgress: null },
  ]) {
    assert.throws(() => validatePenguinIntroEvidence({...valid, introEvidence}), /provenance/);
  }
  assert.throws(() => validatePenguinIntroEvidence({...valid, won: false}), /completed gameplay/);
  assert.throws(() => validatePenguinIntroEvidence({...valid, screenshots: []}), /real screenshot/);
});
