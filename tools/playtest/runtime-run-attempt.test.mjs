import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

// Run the two actual installed attempt-ownership functions, not a substitute.
const source = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const begin = source.indexOf('function createGameRunAttemptGate() {');
const end = source.indexOf('const gameRunAttemptGate = createGameRunAttemptGate();', begin);
assert.ok(begin >= 0 && end > begin, 'missing production attempt-ownership implementation');
const { createGameRunAttemptGate, awaitGameRunAttempt, disposeOwnedGameTextures,
  retireFailedBootstrapAudioIfOwned } = runInNewContext(
  source.slice(begin, end) + '\n({ createGameRunAttemptGate, awaitGameRunAttempt, disposeOwnedGameTextures, retireFailedBootstrapAudioIfOwned });',
  { AbortController, Error }
);

test('run boot lease cancels older attempt before any Pixi app exists, including 128 overlapping starts', () => {
  const gate = createGameRunAttemptGate();
  let prior;
  for (let n = 0; n < 128; n++) {
    const attempt = gate.begin();
    attempt.assertActive();
    assert.equal(attempt.controller.signal.aborted, false);
    if (prior) {
      assert.equal(prior.controller.signal.aborted, true);
      assert.throws(() => prior.assertActive(), { name: 'StaleGameRunError' });
    }
    prior = attempt;
  }
  prior.controller.abort();
  assert.throws(() => prior.assertActive(), { name: 'StaleGameRunError' });
});

test('out-of-order fulfilled network result cannot resurrect the earlier game sheet', async () => {
  const gate = createGameRunAttemptGate();
  const first = gate.begin();
  let finishFirst;
  const delayed = awaitGameRunAttempt(new Promise(resolve => { finishFirst = resolve; }), first);
  const latest = gate.begin();
  const fresh = await awaitGameRunAttempt(Promise.resolve({ gameId: 'new' }), latest);
  assert.equal(fresh.gameId, 'new');
  finishFirst({ gameId: 'retired' });
  await assert.rejects(delayed, { name: 'StaleGameRunError' });
});

test('rejected stale fetch is not presented as the new game load error', async () => {
  const gate = createGameRunAttemptGate();
  const first = gate.begin();
  let rejectFirst;
  const delayed = awaitGameRunAttempt(new Promise((_resolve, reject) => { rejectFirst = reject; }), first);
  gate.begin();
  rejectFirst(Object.assign(new Error('request was aborted'), { name: 'AbortError' }));
  await assert.rejects(delayed, { name: 'StaleGameRunError' });
});

test('current attempt preserves genuine package/network failures', async () => {
  const attempt = createGameRunAttemptGate().begin();
  await assert.rejects(awaitGameRunAttempt(
    Promise.reject(new Error('HTTP 503')), attempt
  ), /HTTP 503/);
});

test('owned texture disposal is idempotent across 128 Play Again sessions and ignores aliases', () => {
  let destroyed = 0;
  for (let attempt = 0; attempt < 128; attempt++) {
    const first = { destroy(all) { assert.equal(all, true); destroyed++; } };
    const second = { destroy(all) { assert.equal(all, true); destroyed++; } };
    const textures = new Map([
      ['a', first], ['alias-of-a', first], ['b', second],
    ]);
    disposeOwnedGameTextures(textures);
    assert.equal(textures.size, 0, 'retired private textures are dereferenced');
    disposeOwnedGameTextures(textures);
    assert.equal(destroyed, (attempt + 1) * 2, 'each private texture is retired once');
  }
});

test('failure to dispose one private texture does not retain its peers', () => {
  let disposedOther = false;
  const textures = new Map([
    ['broken', { destroy() { throw new Error('already released'); } }],
    ['other', { destroy(all) { assert.equal(all, true); disposedOther = true; } }],
  ]);
  disposeOwnedGameTextures(textures);
  assert.equal(disposedOther, true);
  assert.equal(textures.size, 0);
});

test('production boot checks every async ownership boundary and aborts image transfers', () => {
  assert.match(source, /const attempt = gameRunAttemptGate\.begin\(\)/);
  assert.match(source, /await awaitActive\(apiFetch\(\x60\/v1\/games\//);
  assert.match(source, /await awaitActive\(app\.init\(/);
  assert.match(source, /await awaitActive\(Promise\.all\(/);
  assert.match(source, /await awaitActive\(RAPIER\.init\(\)\)/);
  assert.match(source, /abort\.signal\.addEventListener\('abort', onSessionAbort, \{ once: true \}\)/);
  assert.match(source, /attempt\.assertActive\(\);[\s\S]*?const texture = PIXI\.Texture\.from\(img\)/);
  assert.match(source, /if \(err\?\.name === 'StaleGameRunError'\) return;/);
  assert.match(source, /activeSession\.disposeTextures\?\.\(\)/);
  assert.match(source, /activeSession\.disposeTextures = \(\) => disposeOwnedGameTextures\(textures\)/);
  assert.match(source, /disposeOwnedGameTextures\(textures\);/);
});


test('failed Pixi bootstrap resets WebAudio only for the exact owning app', () => {
  const failedApp = {};
  const successorApp = {};
  const observations = [];
  const reset = () => observations.push('audio_generation_released');
  assert.equal(retireFailedBootstrapAudioIfOwned({ app: failedApp }, failedApp, reset), true);
  assert.deepEqual(observations, ['audio_generation_released']);
  assert.equal(retireFailedBootstrapAudioIfOwned({ app: successorApp }, failedApp, reset), false);
  assert.equal(retireFailedBootstrapAudioIfOwned(null, failedApp, reset), false);
  assert.equal(retireFailedBootstrapAudioIfOwned({ app: {} }, failedApp, reset), false);
  assert.equal(observations.length, 1, 'stale failed game must never erase successor audio');
});

test('160 raced failure / next-game ownership interleavings never reset newer audio', () => {
  let audioResets = 0;
  for (let iteration = 0; iteration < 160; iteration++) {
    const oldApp = { iteration, kind: 'old' };
    const newApp = { iteration, kind: 'new' };
    const oldSession = { app: oldApp };
    const reset = () => { audioResets++; };
    assert.equal(retireFailedBootstrapAudioIfOwned(oldSession, oldApp, reset), true);
    // A previous session is now retired; clearing the host reference
    // makes a second asynchronous rejection an idempotent no-op.
    assert.equal(retireFailedBootstrapAudioIfOwned(null, oldApp, reset), false);
    const successor = { app: newApp };
    assert.equal(retireFailedBootstrapAudioIfOwned(successor, oldApp, reset), false);
    assert.equal(retireFailedBootstrapAudioIfOwned(successor, newApp, reset), true);
    assert.equal(retireFailedBootstrapAudioIfOwned({ app: { iteration, kind: 'new' } }, newApp, reset), false,
      'structurally identical attempts may not erase a different app');
    assert.equal(audioResets, (iteration + 1) * 2);
  }
});

test('shipping failed-bootstrap teardown aborts owned WebAudio before retiring WebGL app', () => {
  const start = source.indexOf('const discardFailedBootstrap = (error) => {');
  const end = source.indexOf('// The canvas can outlive an OS background/context-loss event.', start);
  assert.ok(start > 0 && end > start, 'missing actual bootstrap failure path');
  const failurePath = source.slice(start, end);
  assert.match(failurePath, /abort\.abort\(\);[\s\S]*disposeOwnedGameTextures\(textures\);/);
  assert.match(failurePath,
    /if \(retireFailedBootstrapAudioIfOwned\(activeSession, app, \(\) => sfx\.reset\(\)\)\) \{/);
  assert.match(failurePath,
    /sfx\.reset\(\)\)\) \{[\s\S]*app\.destroy\(true, \{ children: true, texture: false \}\)[\s\S]*activeSession = null;/);
  assert.doesNotMatch(failurePath, /^\s*sfx\.reset\(\);/m,
    'an unowned stale failure must never globally reset successor audio');
});

test('failed GamePackage HTTP bodies never enter diagnostics or unbounded buffering (160 hostile cases)', () => {
  const start = source.indexOf('function failClosedPackageHTTPResponse(response) {');
  const end = source.indexOf('\nasync function run(id) {', start);
  assert.ok(start > 0 && end > start, 'test installed fail-closed package error policy');
  const reject = runInNewContext(
    source.slice(start, end) + '\nfailClosedPackageHTTPResponse;',
    { Number, Error }
  );
  for (let i = 0; i < 160; i++) {
    const status = i % 2 ? 403 : 426;
    let cancelled = 0, textReads = 0, jsonReads = 0;
    const maliciousBody = 'private-token-control-' + i + '\\n' + 'x'.repeat(i * 50);
    const response = {
      status,
      body: {
        cancel() {
          cancelled++;
          if (i % 3 === 0) return Promise.reject(new Error(maliciousBody));
          if (i % 3 === 1) throw new Error(maliciousBody);
          return new Promise(() => {}); // a stuck cancellation must never stall error UX
        },
      },
      text() { textReads++; return Promise.resolve(maliciousBody); },
      json() { jsonReads++; return Promise.resolve({ error: maliciousBody }); },
    };
    assert.throws(() => reject(response),
      e => e.message === 'package HTTP ' + status &&
        !e.message.includes(maliciousBody),
      'must report fixed HTTP category without reading or displaying body');
    assert.equal(cancelled, 1, 'unread stream cancellation must be attempted once');
    assert.equal(textReads, 0);
    assert.equal(jsonReads, 0);
  }
  for (const status of ['401 secret', null, -1, NaN, Infinity, 700, {}, undefined]) {
    assert.throws(() => reject({ status, body: null }),
      e => e.message === 'package HTTP 0');
  }
  assert.throws(() => reject({ status: 302, body: null }),
    e => e.message === 'package HTTP 302',
    'unexpected redirect-like response must still fail closed');
});

test('real package HTTP non-success branch never parses raw response text or ID-bearing bodies', () => {
  const start = source.indexOf('const res = await awaitActive(apiFetch(`/v1/games/');
  const end = source.indexOf('const pkg = await awaitActive(res.json());', start);
  assert.ok(start > 0 && end > start, 'must find real run GamePackage fetch');
  const failure = source.slice(start, end);
  assert.match(failure, /if \(!res\.ok\) failClosedPackageHTTPResponse\(res\)/);
  assert.doesNotMatch(failure, /res\.text\(|body\.slice|\.localizedDescription/);
  const helper = source.slice(
    source.indexOf('function failClosedPackageHTTPResponse(response) {'),
    source.indexOf('\nasync function run(id) {')
  );
  assert.match(helper, /response\?\.body\?\.cancel\?\.\(\)/);
  assert.doesNotMatch(helper, /response\.text\(|response\.json\(/);
});

test('GamePackage 200 preflight errors are fixed, allowlisted and non-coercing for 160 hostile values', () => {
  const start = source.indexOf('function safeGamePackagePreflightError(reason) {');
  const end = source.indexOf('\nasync function run(id) {', start);
  assert.ok(start > 0 && end > start, 'shipping Pixi 200-envelope preflight helper must exist');
  const reject = runInNewContext(
    source.slice(start, end) + '\nsafeGamePackagePreflightError;', { Error }
  );
  const expected = {
    identity: 'invalid game package: game identity mismatch',
    format: 'unsupported GamePackage format',
    runtime_version: 'invalid minimum runtime version',
    runtime_incompatible: 'game requires a newer runtime',
    ir_version: 'unsupported Game IR version',
    ir_envelope: 'invalid game package: IR version mismatch',
    capability: 'unsupported or duplicate Game IR capability',
  };
  for (const [reason, message] of Object.entries(expected)) {
    const failure = reject(reason);
    assert.equal(failure.name, 'Error');
    assert.equal(failure.message, message);
  }
  for (let i = 0; i < 160; i++) {
    const privateText = 'sensitive-auth-or-id-' + i + '<script>\r\n' + 'Q'.repeat(i * 30);
    const object = {
      toString() { throw Error(privateText); },
      valueOf() { throw Error(privateText); },
      [Symbol.toPrimitive]() { throw Error(privateText); },
    };
    for (const reason of [privateText, object, null, i, ['identity', privateText]]) {
      const failure = reject(reason);
      assert.equal(failure.message, 'invalid game package');
      assert.ok(!failure.message.includes(privateText));
    }
  }
});

test('shipping GamePackage 200 preflight and diagnostics never print raw untrusted metadata', () => {
  const start = source.indexOf('const res = await awaitActive(apiFetch(`/v1/games/');
  const end = source.indexOf("if (ir.irVersion === '1.2'", start);
  assert.ok(start >= 0 && end > start, 'must inspect real installed run() package preflight');
  const block = source.slice(start, end);
  const expectedReasons = [
    'identity', 'format', 'runtime_version', 'runtime_incompatible',
    'ir_version', 'ir_envelope', 'capability',
  ];
  for (const reason of expectedReasons) {
    assert.match(block, new RegExp("throw safeGamePackagePreflightError\\('" + reason + "'\\)"),
      'real preflight must use fixed reason ' + reason);
  }
  assert.match(block, /log\('PKG', 'package HTTP 200; validating GamePackage compatibility'\)/);
  assert.doesNotMatch(block, /log\('PKG', `/);
  for (const raw of [
    'String(pkg.gameId)', "pkg.packageFormatVersion);", "' + pkg.minRuntimeVersion",
    "' + ir.irVersion", 'String(pkg.irVersion)',
    'String(capability).slice(0, 64)',
  ]) {
    assert.ok(!block.includes(raw), 'raw authored metadata must not reach errors: ' + raw);
  }
  assert.doesNotMatch(block, /throw new Error\(`invalid game package: requested game/);
  // This redaction is intentionally separate from future bounded 200-response
  // transfer / JSON parse admission; it must not weaken existing fail-closed
  // redirect, abort-owned fetch or capability verification.
  assert.match(block, /trustedRuntimeDocumentFetchOptions\(abort\.signal\)/);
  assert.match(block, /if \(!res\.ok\) failClosedPackageHTTPResponse\(res\)/);
  assert.match(block, /const pkg = await awaitActive\(res\.json\(\)\)/);
});
