import test from 'node:test';
import assert from 'node:assert/strict';
import { classifyAssetRequestFailures as classify } from '../playability-kit/asset-failure-classification.mjs';

const base = 'http://localhost:18099';
const asset = { kind: 'image', id: 'bg', url: '/v1/games/g/assets/bg' };
const url = base + asset.url;
const aborted = { url, reason: 'net::ERR_ABORTED', kind: 'fetch' };

test('a redundant image fetch abort is informational only after every texture is proven loaded', () => {
  const result = classify({ requests: [aborted], assets: [asset], texturesLine: [1, 1], base });
  assert.equal(result.redundant.length, 1);
  assert.deepEqual(result.blocking, []);
});

test('failed required texture, unknown URL, or non-aborted errors stay blocking', () => {
  for (const options of [
    { requests: [aborted], assets: [asset], texturesLine: [0, 1], base },
    { requests: [aborted], assets: [asset], texturesLine: null, base },
    { requests: [aborted], assets: [asset, { ...asset, id: 'bg2', url: '/other' }], texturesLine: [1, 2], base },
    { requests: [{ ...aborted, url: base + '/unknown' }], assets: [asset], texturesLine: [1, 1], base },
    { requests: [{ ...aborted, reason: 'net::ERR_CONNECTION_RESET' }], assets: [asset], texturesLine: [1, 1], base },
    { requests: [{ ...aborted, kind: 'image' }], assets: [asset], texturesLine: [1, 1], base },
    { requests: [aborted], assets: [{ kind: 'audio', url: asset.url }], texturesLine: [1, 1], base },
  ]) {
    const result = classify(options);
    assert.equal(result.blocking.length, 1, JSON.stringify(options));
    assert.equal(result.redundant.length, 0);
  }
});


const soundAsset = { kind: 'audio', id: 'theme', url: '/v1/games/g/assets/theme' };
const soundURL = base + soundAsset.url;
const soundAbort = { url: soundURL, reason: 'net::ERR_ABORTED', kind: 'fetch' };
const soundEvidence = () => ({
  network: [{ id: 12, url: soundURL, phase: 'failed', reason: 'net::ERR_ABORTED' }],
  activeDecodeJobs: 0,
  decodedSoundIds: ['theme'],
  events: [
    { event: 'request_started', assetId: 'theme', atMs: 319 },
    { event: 'response_200', assetId: 'theme', atMs: 355 },
    { event: 'decode_ok', assetId: 'theme', atMs: 552 },
  ],
});

test('one unique aborted audio request may be informational only with same-session complete decoded PCM', () => {
  const result = classify({
    requests: [soundAbort], assets: [soundAsset], texturesLine: null, base,
    audioEvidence: soundEvidence(),
  });
  assert.deepEqual(result.blocking, []);
  assert.equal(result.decodedAudio.length, 1);
  assert.equal(result.redundant.length, 0);
});

test('missing, retried, unauthorized or interrupted audio cannot masquerade as a decoded request', () => {
  const baseEvidence = soundEvidence();
  for (const evidence of [
    null,
    { ...baseEvidence, decodedSoundIds: [] },
    { ...baseEvidence, activeDecodeJobs: 1 },
    { ...baseEvidence, network: [] },
    { ...baseEvidence, network: [...baseEvidence.network, { ...baseEvidence.network[0], id: 13 }] },
    { ...baseEvidence, network: [{ ...baseEvidence.network[0], phase: 'finished' }] },
    { ...baseEvidence, events: baseEvidence.events.filter(e => e.event !== 'decode_ok') },
    { ...baseEvidence, events: baseEvidence.events.filter(e => e.event !== 'response_200') },
    { ...baseEvidence, events: [...baseEvidence.events, { event: 'request_started', assetId: 'theme', atMs: 553 }] },
    { ...baseEvidence, events: [...baseEvidence.events, { event: 'session_reset', assetId: '', atMs: 554 }] },
    { ...baseEvidence, events: [...baseEvidence.events, { event: 'timeout_abort', assetId: 'theme', atMs: 550 }] },
    { ...baseEvidence, events: [...baseEvidence.events, { event: 'fetch_or_decode_error_AbortError', assetId: 'theme', atMs: 551 }] },
    { ...baseEvidence, events: [{ ...baseEvidence.events[0], atMs: 554 }, ...baseEvidence.events.slice(1)] },
  ]) {
    const result = classify({
      requests: [soundAbort], assets: [soundAsset], texturesLine: null, base,
      audioEvidence: evidence,
    });
    assert.equal(result.blocking.length, 1, 'must retain blocking: ' + JSON.stringify(evidence));
    assert.equal(result.decodedAudio.length, 0);
  }
  for (const options of [
    { requests: [{ ...soundAbort, reason: 'net::ERR_CONNECTION_RESET' }], assets: [soundAsset] },
    { requests: [soundAbort], assets: [] },
    { requests: [soundAbort], assets: [{ ...soundAsset, id: 'wrong' }] },
    { requests: [soundAbort], assets: [{ ...soundAsset, url: '/v1/games/other/assets/theme' }] },
    { requests: [soundAbort], assets: [soundAsset, { ...soundAsset, id: 'duplicate' }] },
    { requests: [{ ...soundAbort, kind: 'image' }], assets: [soundAsset] },
  ]) {
    const result = classify({
      ...options, texturesLine: null, base, audioEvidence: baseEvidence,
    });
    assert.equal(result.blocking.length, 1, 'must retain blocking: ' + JSON.stringify(options));
  }
});
