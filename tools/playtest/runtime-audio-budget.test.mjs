import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const source = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('WebAudio is isolated from previous game version and canceled on restart/pagehide', () => {
  assert.match(source, /if \(activeSession\) \{[^]*?sfx\.reset\(\);/);
  assert.match(source, /addEventListener\('pagehide', \(\) => sfx\.reset\(\)\)/);
  assert.match(source, /this\.generation\+\+/);
  assert.match(source, /this\.abort\.abort\(\)/);
  assert.match(source, /this\.buffers\.clear\(\)/);
  assert.match(source, /this\.inFlight\.clear\(\)/);
  assert.match(source, /previous\.close\(\)\.catch/);
  assert.ok(source.includes("const key = assetId + '|' + url"));
  assert.doesNotMatch(source, /this\.buffers\.set\(assetId, buffer\)/);
});

test('audio does not bypass protected fetch and finite decoded PCM budgets', () => {
  assert.match(source, /MAX_AUDIO_TRANSFER_BYTES = 8 \* 1024 \* 1024/);
  assert.match(source, /MAX_TOTAL_AUDIO_DECODED_BYTES = 32 \* 1024 \* 1024/);
  assert.match(source, /MAX_ACTIVE_SOUND_SOURCES = 32/);
  assert.match(source, /MAX_QUEUED_SOUND_JOBS = 32/);
  assert.match(source, /this\.decodedBytes \+ decodedBytes > MAX_TOTAL_AUDIO_DECODED_BYTES/);
  assert.match(source, /apiFetch\(url, \{ signal: request\.signal \}\)/);
  assert.doesNotMatch(source, /new Audio\(url\)/);
});

test('duplicate audio events share one bounded in-flight fetch/decode', () => {
  assert.match(source, /MAX_CONCURRENT_AUDIO_DECODES = 4/);
  assert.match(source, /AUDIO_FETCH_TIMEOUT_MS = 15_000/);
  assert.match(source, /let pending = this\.inFlight\.get\(key\)/);
  assert.match(source, /this\.inFlight\.size >= MAX_CONCURRENT_AUDIO_DECODES/);
  assert.match(source, /this\.inFlight\.set\(key, pending\)/);
  assert.match(source, /if \(this\.inFlight\.get\(key\) === pending\) this\.inFlight\.delete\(key\)/);
  assert.match(source, /const timeout = setTimeout\(\(\) => request\.abort\(\), AUDIO_FETCH_TIMEOUT_MS\)/);
  assert.match(source, /sessionSignal\.addEventListener\('abort', onAbort, \{ once: true \}\)/);
  assert.match(source, /clearTimeout\(timeout\)/);
});
