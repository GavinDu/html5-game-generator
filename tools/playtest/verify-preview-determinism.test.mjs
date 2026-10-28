import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const source = await readFile(fileURLToPath(new URL('./verify-preview-determinism.mjs', import.meta.url)), 'utf8');

test('determinism verifier renders the same recorded run twice with the same options', () => {
  assert.match(source, /render\('first'\)/);
  assert.match(source, /render\('second'\)/);
  assert.match(source, /'--run', run/);
  assert.match(source, /'--duration-ms', durationMs, '--fps', fps/);
});

test('determinism verifier rejects fallback clips and incomplete frame reports', () => {
  assert.match(source, /!report\.clip \|\| report\.clip_fallback/);
  assert.match(source, /!Array\.isArray\(report\.frame_sha256\)/);
  assert.match(source, /did not produce a complete clip with frame hashes/);
});

test('determinism verifier exits unsuccessfully when any frame hash differs', () => {
  assert.match(source, /compareFrameHashes\(first\.frame_sha256, second\.frame_sha256\)/);
  assert.match(source, /if \(!comparison\.match\) process\.exitCode = 1/);
});

test('determinism verifier cleans temporary render artifacts on both success and failure', () => {
  assert.match(source, /finally \{/);
  assert.match(source, /rmSync\(temp, \{ recursive: true, force: true \}\)/);
});
