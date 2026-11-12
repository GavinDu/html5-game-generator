import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
const source = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('WebAudio lifecycle belongs to game sessions and clears on restart/pagehide', () => {
  assert.match(source, /if \(activeSession\) \{[^]*?sfx\.reset\(\);/);
  assert.match(source, /addEventListener\('pagehide', \(\) => sfx\.reset\(\)\)/);
  assert.match(source, /this\.generation\+\+/);
  assert.match(source, /this\.abort\.abort\(\)/);
  assert.match(source, /this\.buffers\.clear\(\)/);
  assert.match(source, /this\.inFlight\.clear\(\)/);
  assert.match(source, /previous\.close\(\)\.catch/);
  assert.ok(source.includes("const key = assetId + '|' + url"));
});

test('audio asset transfer and decoded resource budgets never bypass authenticated fetch', () => {
  assert.match(source, /MAX_AUDIO_TRANSFER_BYTES = 8 \* 1024 \* 1024/);
  assert.match(source, /MAX_TOTAL_AUDIO_DECODED_BYTES = 32 \* 1024 \* 1024/);
  assert.match(source, /MAX_ACTIVE_SOUND_SOURCES = 32/);
  assert.match(source, /MAX_QUEUED_SOUND_JOBS = 32/);
  assert.match(source, /this\.decodedBytes \+ decodedBytes > MAX_TOTAL_AUDIO_DECODED_BYTES/);
  assert.match(source, /apiFetch\(url, \{ signal: request\.signal \}\)/);
  const executable = source.slice(source.indexOf('const sfx = {'), source.indexOf('const EASINGS = {'));
  assert.doesNotMatch(executable, /new Audio\(url\)/);
});

test('audio fetch/decode concurrency is deduplicated and has a deadline', () => {
  assert.match(source, /MAX_CONCURRENT_AUDIO_DECODES = 4/);
  assert.match(source, /AUDIO_FETCH_TIMEOUT_MS = 15_000/);
  assert.match(source, /let pending = this\.inFlight\.get\(key\)/);
  assert.match(source, /this\.inFlight\.size >= MAX_CONCURRENT_AUDIO_DECODES/);
  assert.match(source, /this\.inFlight\.set\(key, pending\)/);
  assert.match(source, /if \(this\.inFlight\.get\(key\) === pending\) this\.inFlight\.delete\(key\)/);
  assert.match(source, /const timeout = setTimeout\(\(\) => \{ this\.trace\('timeout_abort', assetId\); request\.abort\(\); \}, AUDIO_FETCH_TIMEOUT_MS\)/);
});

test('audio URLs are validated before any authenticated fetch, including preloads', () => {
  const play = source.indexOf('play(url, assetId, preloadOnly = false)');
  const sameOrigin = source.indexOf('requested.origin !== location.origin', play);
  const authFetch = source.indexOf('apiFetch(url, { signal: request.signal })', play);
  assert.ok(play > 0 && sameOrigin > play && authFetch > sameOrigin,
    'reject cross-origin audio before fetching it with application credentials');
  assert.match(source, /throw new Error\('audio asset URL must be same-origin'\)/);
});

test('bounded high-use predecode is nonblocking and never starts playback', () => {
  assert.match(source, /MAX_SPECULATIVE_AUDIO_PREFETCH = 2/);
  assert.match(source, /\.slice\(0, MAX_SPECULATIVE_AUDIO_PREFETCH\)/);
  assert.match(source, /sfx\.play\(audio\.get\(assetId\), assetId, true\)/);
  assert.match(source, /if \(ctx\.state === 'suspended' && !preloadOnly\)/);
  assert.match(source, /if \(buffer && !preloadOnly\) start\(buffer\)/);
  assert.match(source, /decodedSoundIds: \[\.\.\.sfx\.buffers\.keys\(\)\]/);
});
