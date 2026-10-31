import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const runtimePath = fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url));
const source = await readFile(runtimePath, 'utf8');

test('private image assets are fetched through the authenticated same-origin API client', () => {
  assert.match(source, /assetURL\.origin !== location\.origin/);
  assert.match(source, /apiFetch\(assetURL\.href\)/);
  assert.match(source, /image asset URL must be same-origin/);
});

test('image responses are checked before decoding and texture creation', () => {
  assert.match(source, /if \(!response\.ok\)/);
  assert.match(source, /image asset request failed \(HTTP/);
  assert.match(source, /URL\.createObjectURL\(await response\.blob\(\)\)/);
  assert.match(source, /return PIXI\.Texture\.from\(img\)/);
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
