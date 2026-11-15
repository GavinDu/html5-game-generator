import test from 'node:test';
import assert from 'node:assert/strict';
import {
  provisionalRunPlacementLabel,
  mayRequestNativeRunVerificationLookup,
} from '../../internal/api/web/runtime-result-provenance.mjs';

test('browser result placement always says provisional, never verified', () => {
  const label = provisionalRunPlacementLabel({
    runId: 'untrusted', rank: 1, rankingScope: 'provisional',
    evidenceStatus: 'unverified', verified: false,
  });
  assert.equal(label, 'Provisional leaderboard position #1 this week (unverified)');
  assert.match(label, /Provisional/);
  assert.match(label, /unverified/);
  assert.equal(provisionalRunPlacementLabel({
    rank: 99, rankingScope: 'provisional', verified: true,
    evidenceStatus: 'verified', attested: true,
  }), 'Provisional leaderboard position #99 this week (unverified)',
  'a spoofed boolean cannot promote a provisional position');

  const old = provisionalRunPlacementLabel({ rank: 42, verified: true });
  assert.equal(old, 'Provisional leaderboard position #42 this week (unverified)',
    'missing provenance is always legacy/provisional, never trusted');
});

test('reject invalid or unknown unverified rank shapes without invented placement', () => {
  for (const rank of [undefined, null, 0, -1, 1.5, '1', true, NaN, Infinity,
    Number.MAX_SAFE_INTEGER, 1_000_001]) {
    assert.equal(provisionalRunPlacementLabel({
      rank, rankingScope: 'provisional',
    }), null, `bad rank ${String(rank)} should be invisible`);
  }
  for (const bad of [null, 0, '', [], {}, false,
    { rank: 1, rankingScope: 'verified_only' },
    { rank: 1, rankingScope: 'authoritative' },
    { rank: 1, rankingScope: null },
    { rank: 1, rankingScope: '' },
    { rank: 1, rankingScope: ['provisional'] }]) {
    assert.equal(provisionalRunPlacementLabel(bad), null);
  }
});

test('native run notification requires full server-attested verdict shape', () => {
  const valid = {
    runId: 'server-attempt-1', verified: true, attested: true,
    rankingScope: 'verified_only', evidenceStatus: 'verified',
  };
  assert.equal(mayRequestNativeRunVerificationLookup(valid), true,
    'still only a lookup hint; native separately validates the run');
  for (const [key, value] of [
    ['verified', false], ['attested', false],
    ['rankingScope', 'provisional'], ['evidenceStatus', 'unverified'],
    ['runId', ''], ['runId', 'x'.repeat(129)], ['runId', 1],
  ]) {
    assert.equal(mayRequestNativeRunVerificationLookup({ ...valid, [key]: value }),
      false, `unsafe ${key} should not notify native`);
  }
  for (const legacy of [
    null, false, {}, { runId: 'browser', verified: true },
    { runId: 'browser', verified: true, attested: true },
  ]) {
    assert.equal(mayRequestNativeRunVerificationLookup(legacy), false);
  }
});
