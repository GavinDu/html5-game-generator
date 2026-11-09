import test from 'node:test';
import assert from 'node:assert/strict';
import { evaluateFeedPreviewReadiness } from './feed-media-readiness.mjs';

const viewport = (type) => ({
  passed: true,
  beforePreview: { type, valid: true },
  afterPreview: { type, valid: true },
});

test('honest placeholder is valid UX but NOT media-ready', () => {
  const result = evaluateFeedPreviewReadiness(Array.from({length: 4}, () => viewport('honest-placeholder')));
  assert.equal(result.status, 'needs-reviewed-gameplay-media');
  assert.equal(result.honestFallbackCount, 8);
  assert.equal(result.posterBacked, false);
  assert.equal(result.clipVerified, false);
});

test('verified still posters are not falsely claimed as preview clips', () => {
  const result = evaluateFeedPreviewReadiness(Array.from({length: 4}, () => viewport('verified-poster')));
  assert.equal(result.status, 'poster-backed-clip-unverified');
  assert.equal(result.verifiedPosterCount, 8);
  assert.equal(result.posterBacked, true);
  assert.equal(result.clipVerified, false);
});

test('missing profiles or broken media stay unready', () => {
  assert.equal(evaluateFeedPreviewReadiness([viewport('verified-poster')]).posterBacked, false);
  const mixed = Array.from({length: 4}, () => viewport('verified-poster'));
  mixed[0].afterPreview = { type: 'unexplained-blank', valid: false };
  assert.equal(evaluateFeedPreviewReadiness(mixed).unknownOrBrokenCount, 1);
  assert.equal(evaluateFeedPreviewReadiness(mixed).posterBacked, false);
});
