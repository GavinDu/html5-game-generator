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
