import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';

const runtimePath = fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url));
const source = await readFile(runtimePath, 'utf8');

test('private image assets are fetched through the authenticated same-origin API client', () => {
  assert.match(source, /assetURL\.origin !== location\.origin/);
  assert.match(source, /apiFetch\(assetURL\.href,\s*authenticatedAssetFetchOptions\(requestController\.signal\)\)/);
  assert.match(source, /image asset URL must be same-origin/);
});

test('image responses are checked before decoding and texture creation', () => {
  assert.match(source, /if \(!response\.ok\)/);
  assert.match(source, /image asset request failed \(HTTP/);
  assert.match(source, /imageBlob = await readBoundedImageBlob\(response\)/);
  assert.match(source, /const blob = await response\.blob\(\)/);
  assert.match(source, /URL\.createObjectURL\(imageBlob\)/);
  assert.match(source, /const texture = PIXI\.Texture\.from\(img\)/);
  assert.match(source, /return texture/);
});

test('failed required image assets fail the launch instead of silently dropping sprites', () => {
  assert.match(source, /required image asset '\$\{asset\.id\}' failed validation or loading/);
  assert.doesNotMatch(source, /log\('PKG', `WARN asset '\$\{asset\.id\}' failed to load/);
});

test('temporary object URLs are revoked after image decoding', () => {
  assert.match(source, /finally \{\s*URL\.revokeObjectURL\(objectURL\);\s*\}/);
});

test('package envelope identity, IR version, and asset delivery entries are validated before launch', () => {
  assert.match(source, /pkg\.gameId !== id/);
  assert.match(source, /pkg\.irVersion !== ir\.irVersion/);
  assert.match(source, /Array\.isArray\(pkg\.assets\)/);
  assert.match(source, /duplicate asset ID/);
  assert.match(source, /IR asset '\$\{asset\.id\}' has no delivery entry/);
});

test('declared image SHA-256 hashes are verified before decode', () => {
  assert.match(source, /asset\.sha256/);
  assert.match(source, /crypto\.subtle\.digest\('SHA-256'/);
  assert.match(source, /image asset integrity check failed/);
  assert.match(source, /invalid SHA-256 declaration/);
});


test('actual installed private asset policy denies redirects while preserving source abort control', () => {
  const begin = source.indexOf('function authenticatedAssetFetchOptions(signal) {');
  const end = source.indexOf('async function apiFetch(input, init = {}) {', begin);
  assert.ok(begin > 0 && end > begin, 'must test the real shipped asset fetch options helper');
  const policy = runInNewContext(
    source.slice(begin, end) + '\nauthenticatedAssetFetchOptions;',
    { Object, TypeError }
  );
  for (let i = 0; i < 160; i++) {
    const controller = new AbortController();
    const options = policy(controller.signal);
    assert.equal(options.signal, controller.signal, 'one abort controller owns the full transfer');
    assert.equal(options.redirect, 'error');
    assert.equal(Object.isFrozen(options), true);
    assert.equal(new Request('https://origin.example.test/v1/assets/' + i, options).redirect, 'error');
    controller.abort();
    assert.equal(options.signal.aborted, true);
  }
  for (const invalid of [null, undefined, {}, { aborted: false }]) {
    assert.throws(() => policy(invalid), TypeError);
  }
});

test('neither private image nor WebAudio transfer can silently follow HTTP redirects', () => {
  const imageStart = source.indexOf('async function loadTexture(asset) {');
  const audioStart = source.indexOf('play(url, assetId, preloadOnly = false) {');
  assert.ok(imageStart > 0 && audioStart > 0);
  const image = source.slice(imageStart, source.indexOf('const objectURL = URL.createObjectURL(imageBlob)', imageStart));
  const audio = source.slice(audioStart, source.indexOf('const EASINGS = {', audioStart));
  assert.match(image, /apiFetch\(assetURL\.href,\s*authenticatedAssetFetchOptions\(requestController\.signal\)\)/);
  assert.match(audio, /apiFetch\(url,\s*authenticatedAssetFetchOptions\(request\.signal\)\)/);
  assert.match(source, /return Object\.freeze\(\{ signal, redirect: 'error' \}\)/);
  assert.doesNotMatch(image, /apiFetch\(assetURL\.href,\s*\{\s*signal:/);
  assert.doesNotMatch(audio, /apiFetch\(url,\s*\{\s*signal:/);
  // Keep separate initial-origin/authorization restrictions. This only
  // closes the later redirect escape, not server package authenticity.
  assert.match(image, /assetURL\.origin !== location\.origin/);
  assert.match(audio, /requested\.origin !== location\.origin/);
});

test('trusted game-document fetches reject redirects at both shipped package and replay read sites', () => {
  const replayStart = source.indexOf('const rr = await awaitActive(apiFetch(`/v1/runs/');
  const packageStart = source.indexOf('const res = await awaitActive(apiFetch(`/v1/games/');
  assert.ok(replayStart > 0 && packageStart > replayStart,
    'must locate real run-replay and game-package Fetch callsites');
  const replay = source.slice(replayStart, source.indexOf('\n', replayStart));
  const pkg = source.slice(packageStart, source.indexOf('\n', packageStart));
  assert.match(replay, /\/v1\/runs\//);
  assert.match(pkg, /\/package`/);
  for (const site of [replay, pkg]) {
    assert.match(site, /trustedRuntimeDocumentFetchOptions\(abort\.signal\)/);
    assert.doesNotMatch(site, /\{\s*signal:\s*abort\.signal\s*\}/,
      'never allow implicit Fetch redirect:follow for trusted game JSON');
  }
  assert.equal(source.match(/trustedRuntimeDocumentFetchOptions\(abort\.signal\)/g)?.length, 2);
});

test('installed game-document fetch policy forwards a sealed no-redirect signal through the real API wrapper', async () => {
  const start = source.indexOf('function authenticatedAssetFetchOptions(signal) {');
  const end = source.indexOf('function setAccountStatus(message) {', start);
  assert.ok(start > 0 && end > start);
  const bridge = runInNewContext(
    source.slice(start, end) +
      '\n({ policy: trustedRuntimeDocumentFetchOptions, fetch: apiFetch });',
    {
      URL, Headers, Object, TypeError,
      location: {
        href: 'https://game.example.test/player?game=sample',
        origin: 'https://game.example.test',
      },
      readSessionToken: () => 'private-test-token',
      window: { fetch: async (_url, init) => init },
    }
  );
  for (let i = 0; i < 160; i++) {
    const controller = new AbortController();
    const options = bridge.policy(controller.signal);
    assert.equal(options.signal, controller.signal);
    assert.equal(Object.isFrozen(options), true);
    assert.equal(options.redirect, 'error');
    const resource = i % 2 === 0
      ? 'https://game.example.test/v1/games/verified_game/package'
      : 'https://game.example.test/v1/runs/verified_run';
    assert.equal(new Request(resource, options).redirect, 'error');
    const forwarded = await bridge.fetch(resource, options);
    assert.equal(forwarded.redirect, 'error', 'apiFetch must preserve redirect policy');
    assert.equal(forwarded.signal, controller.signal);
    assert.equal(forwarded.headers.get('Authorization'), 'Bearer private-test-token');
    controller.abort();
    assert.equal(options.signal.aborted, true);
  }
  for (const invalid of [undefined, null, {}, { aborted: false }]) {
    assert.throws(() => bridge.policy(invalid), TypeError);
  }
});
