import test from 'node:test';
import assert from 'node:assert/strict';
import { validatePenguinIntroEvidence, validateBoboMidplayEvidence } from './preview-evidence.mjs';

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


const boboReport = {
  passed: true,
  gesture: 'real touchscreen tap',
  haloFollowsPlayer: true,
  croppedBrownPixels: 0,
  before: { x: 195 }, after: { x: 319 },
  screenshots: ['before-short-tap.png', 'midplay-restored-rainbow.png'],
  midplayEvidence: {
    source: 'guided real touch input',
    screenshot: 'midplay-restored-rainbow.png',
    progress: 4, score: 90, playerX: 195,
  },
};

test('Bobo poster must use real centered and uncropped rainbow-restoration gameplay', () => {
  assert.deepEqual(validateBoboMidplayEvidence(boboReport), boboReport.midplayEvidence);
  assert.deepEqual(validateBoboMidplayEvidence({
    ...boboReport, midplayEvidence: {...boboReport.midplayEvidence, progress: 11},
  }).progress, 11);
});

test('Bobo untouched start, end screen, missing screenshot and clipped source art fail closed', () => {
  for (const changes of [
    { midplayEvidence: {...boboReport.midplayEvidence, progress: 0} },
    { midplayEvidence: {...boboReport.midplayEvidence, progress: 12} },
    { midplayEvidence: {...boboReport.midplayEvidence, playerX: 385} },
    { midplayEvidence: {...boboReport.midplayEvidence, source: 'mocked' } },
    { midplayEvidence: null },
  ]) {
    assert.throws(() => validateBoboMidplayEvidence({...boboReport, ...changes}), /provenance/);
  }
  assert.throws(() => validateBoboMidplayEvidence({...boboReport, croppedBrownPixels: 30}), /fidelity/);
  assert.throws(() => validateBoboMidplayEvidence({...boboReport, screenshots: []}), /screenshot/);
  assert.throws(() => validateBoboMidplayEvidence({...boboReport, passed: false}), /passed/);
  assert.throws(() => validateBoboMidplayEvidence({...boboReport, gesture: 'simulated'}), /real-touch/);
});
