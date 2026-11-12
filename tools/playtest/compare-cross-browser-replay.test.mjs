import test from 'node:test';
import assert from 'node:assert/strict';
import { compareCrossBrowserReplay } from './compare-cross-browser-replay.mjs';

const example = (browser) => ({
  browser,
  passed: true,
  results: [
    { passed: true, viewport: 'iphone-portrait', seed: 271828, simStep: 600,
      state: { step: 600, score: 3, ended: false, won: false, gap: 175.125,
        lane: 1, entities: [{ id: 'player', x: 195, y: 360.625 }] } },
    { passed: true, viewport: 'ipad-portrait', seed: 271828, simStep: 600,
      state: { step: 600, score: 3, ended: false, won: false, gap: 175.125,
        lane: 1, entities: [{ id: 'player', x: 195, y: 360.625 }] } },
  ],
});

test('identical fixed tick and semantic states across browsers pass', () => {
  const report = compareCrossBrowserReplay(example('chromium'), example('webkit'));
  assert.equal(report.passed, true);
  assert.equal(report.matched.length, 2);
});

test('real physics or score divergence fails instead of accepting approximate screenshots', () => {
  const webkit = example('webkit');
  webkit.results[0].state.entities[0].x += 0.001;
  assert.throws(() => compareCrossBrowserReplay(example('chromium'), webkit),
    /diverged between Chromium and WebKit/);
});

test('missing, duplicate or inconclusive profile evidence fails closed', () => {
  const incomplete = example('webkit');
  incomplete.results.pop();
  assert.throws(() => compareCrossBrowserReplay(example('chromium'), incomplete),
    /profile list differs/);

  const duplicate = example('webkit');
  duplicate.results.push(structuredClone(duplicate.results[0]));
  assert.throws(() => compareCrossBrowserReplay(example('chromium'), duplicate),
    /duplicate viewport/);

  const unverifiable = example('webkit');
  unverifiable.results[0].passed = false;
  assert.throws(() => compareCrossBrowserReplay(example('chromium'), unverifiable),
    /failed profile/);
});

test('same state at a different simulation step is not a determinism proof', () => {
  const webkit = example('webkit');
  webkit.results[0].simStep += 1;
  assert.throws(() => compareCrossBrowserReplay(example('chromium'), webkit),
    /state not captured at reported simulation step/);
});
