import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

// Run the two actual installed attempt-ownership functions, not a substitute.
const source = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const begin = source.indexOf('function createGameRunAttemptGate() {');
const end = source.indexOf('const gameRunAttemptGate = createGameRunAttemptGate();', begin);
assert.ok(begin >= 0 && end > begin, 'missing production attempt-ownership implementation');
const { createGameRunAttemptGate, awaitGameRunAttempt, disposeOwnedGameTextures } = runInNewContext(
  source.slice(begin, end) + '\n({ createGameRunAttemptGate, awaitGameRunAttempt, disposeOwnedGameTextures });',
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
