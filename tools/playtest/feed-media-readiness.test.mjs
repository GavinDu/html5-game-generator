import test from 'node:test';
import assert from 'node:assert/strict';
import { evaluateFeedPreviewReadiness, hasCompleteViewportCoverage, REQUIRED_FEED_VIEWPORTS } from './feed-media-readiness.mjs';

const viewport = (type, name) => ({
  viewport: name,
  passed: true,
  beforePreview: { type, valid: true },
  afterPreview: { type, valid: true },
});
const fourViewports = type => REQUIRED_FEED_VIEWPORTS.map(name => viewport(type, name));

test('honest placeholder is valid UX but NOT media-ready', () => {
  const result = evaluateFeedPreviewReadiness(fourViewports('honest-placeholder'));
  assert.equal(result.status, 'needs-reviewed-gameplay-media');
  assert.equal(result.honestFallbackCount, 8);
  assert.equal(result.posterBacked, false);
  assert.equal(result.clipVerified, false);
});

test('verified still posters are not falsely claimed as preview clips', () => {
  const result = evaluateFeedPreviewReadiness(fourViewports('verified-poster'));
  assert.equal(result.status, 'poster-backed-clip-unverified');
  assert.equal(result.verifiedPosterCount, 8);
  assert.equal(result.posterBacked, true);
  assert.equal(result.clipVerified, false);
});

test('missing profiles or broken media stay unready', () => {
  assert.equal(evaluateFeedPreviewReadiness([viewport('verified-poster')]).posterBacked, false);
  const mixed = fourViewports('verified-poster');
  mixed[0].afterPreview = { type: 'unexplained-blank', valid: false };
  assert.equal(evaluateFeedPreviewReadiness(mixed).unknownOrBrokenCount, 1);
  assert.equal(evaluateFeedPreviewReadiness(mixed).posterBacked, false);
});


test('missing, duplicate or mislabeled device profiles cannot claim screenshot coverage', () => {
  const correct = fourViewports('verified-poster');
  assert.equal(hasCompleteViewportCoverage(correct), true);
  assert.equal(evaluateFeedPreviewReadiness(correct).posterBacked, true);
  const duplicate = correct.map((v, i) => i === 3 ? {...v, viewport: 'ipad-portrait'} : v);
  assert.equal(hasCompleteViewportCoverage(duplicate), false);
  assert.equal(evaluateFeedPreviewReadiness(duplicate).posterBacked, false);
  assert.equal(evaluateFeedPreviewReadiness(duplicate).viewportCoverageComplete, false);
  assert.match(evaluateFeedPreviewReadiness(duplicate).notes, /missing or duplicated/);
  const unexpected = correct.map((v, i) => i === 3 ? {...v, viewport: 'desktop'} : v);
  assert.equal(hasCompleteViewportCoverage(unexpected), false);
  assert.equal(hasCompleteViewportCoverage(correct.slice(0, 3)), false);
});
