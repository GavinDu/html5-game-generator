import test from 'node:test';
import assert from 'node:assert/strict';
import { previewCaptureStep } from './capture-window.mjs';

test('default capture still begins at simulation step zero', () => {
  assert.equal(previewCaptureStep(0, 12), 0);
  assert.equal(previewCaptureStep(1, 12), 5);
  assert.equal(previewCaptureStep(71, 12), 355);
});

test('approved seven-second offset lands on replay step 420 and preserves frame cadence', () => {
  assert.equal(previewCaptureStep(0, 12, 7000), 420);
  assert.equal(previewCaptureStep(1, 12, 7000), 425);
  assert.equal(previewCaptureStep(71, 12, 7000), 775);
  assert.equal(previewCaptureStep(0, 12, 7000), previewCaptureStep(0, 12, 7000));
});

test('unsafe or ambiguous offsets never become a successful preview', () => {
  for (const ms of [-1, 30001, 1.5, Number.NaN, Number.POSITIVE_INFINITY]) {
    assert.throws(() => previewCaptureStep(0, 12, ms), RangeError);
  }
  assert.throws(() => previewCaptureStep(-1, 12, 0), RangeError);
  assert.throws(() => previewCaptureStep(0, 31, 0), RangeError);
  assert.throws(() => previewCaptureStep(0, 0, 0), RangeError);
});
