import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

const source = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const begin = source.indexOf('function buildNativeBridgeMessage(');
const end = source.indexOf('\nfunction sendNativeHostMessage(', begin);
assert.ok(begin >= 0 && end > begin, 'must extract the installed shipping message builder');
const build = runInNewContext('(' + source.slice(begin, end).trim() + ')');

const id = 'current-game', session = 'runtime-session-1';
const nonce = '0123456789abcdef'.repeat(2);
function plain(value) { return value === null ? null : JSON.parse(JSON.stringify(value)); }

test('old native host keeps schema v1 without a v2 capability advertisement', () => {
  const metric = plain(build('player_metric', { metric: 'first_input', sequence: 1, ms: 20 },
    undefined, session, id));
  assert.deepEqual(metric, {
    metric: 'first_input', sequence: 1, ms: 20, schema: 1,
    type: 'player_metric', sessionId: session, gameId: id,
  });
  assert.equal(Object.hasOwn(metric, 'documentNonce'), false);
  const verified = plain(build('verified_run_available', { runId: 'opaque' }, undefined, session, id));
  assert.equal(verified.schema, 1);
  assert.equal(verified.runId, 'opaque');
});

test('native-installed v2 challenge is echoed on metrics and verified-run notifications', () => {
  const bridge = { schema: 2, documentNonce: nonce };
  for (const [type, fields] of [
    ['player_metric', { metric: 'first_scene_render_submit', sequence: 4, ms: 100 }],
    ['player_metric', { metric: 'perf_window', sequence: 5,
      frameTimeP95Ms: 20, frameTimeAvgMs: 16, frameCount: 60 }],
    ['verified_run_available', { runId: 'server-run-id' }],
  ]) {
    const result = plain(build(type, fields, bridge, session, id));
    assert.equal(result.schema, 2);
    assert.equal(result.documentNonce, nonce);
    assert.equal(result.type, type);
    assert.equal(result.sessionId, session);
    assert.equal(result.gameId, id);
    for (const [key, val] of Object.entries(fields)) assert.equal(result[key], val);
  }
});

test('malformed or present-but-invalid v2 bridge never silently downgrades to v1', () => {
  for (const invalid of [
    null, false, '', {}, { schema: 1, documentNonce: nonce },
    { schema: 2 }, { schema: 2, documentNonce: null },
    { schema: 2, documentNonce: 'a'.repeat(31) },
    { schema: 2, documentNonce: 'A'.repeat(32) },
    { schema: 2, documentNonce: '0'.repeat(64) },
    { schema: 2, documentNonce: 'f'.repeat(31) + 'g' },
  ]) {
    assert.equal(build('player_metric', { metric: 'first_input', sequence: 1, ms: 10 },
      invalid, session, id), null);
  }
});

test('unknown command and attempts to override privileged bridge metadata are rejected', () => {
  assert.equal(build('native_control', {}, undefined, session, id), null);
  assert.equal(build('player_metric', null, undefined, session, id), null);
  assert.equal(build('player_metric', [], undefined, session, id), null);
  for (const field of ['schema', 'type', 'sessionId', 'gameId', 'documentNonce']) {
    assert.equal(build('player_metric', { metric: 'first_input', sequence: 1, [field]: 'spoof' },
      { schema: 2, documentNonce: nonce }, session, id), null);
  }
});

test('shipping runtime uses the same builder for all narrow messages without a native control API', () => {
  assert.match(source, /sendNativeHostMessage\('player_metric', \{ metric, sequence:/);
  assert.match(source, /if \(data\.verified === true\) \{[\s\S]*?sendNativeHostMessage\('verified_run_available', \{ runId: data\.runId \}\)/);
  assert.match(source, /if \(!payload\) return; \/\/ malformed installed bridge: no v1 fallback/);
  assert.match(source, /window\.webkit\?\.messageHandlers\?\.hephaestusRunResult\?\.postMessage\(payload\)/);
  assert.doesNotMatch(source, /postMessage\(\{\s*schema: 1,\s*type: 'verified_run_available'/);
});
