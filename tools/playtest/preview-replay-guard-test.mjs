#!/usr/bin/env node
// Browser-CI contract: an absent recording MUST fail before any screenshot
// can be emitted or misattributed to an unrelated run.
import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const arg = (name, fallback) => {
  const i = process.argv.indexOf('--' + name);
  return i >= 0 ? process.argv[i + 1] : fallback;
};
const base = arg('base', 'http://localhost:18099');
const browser = arg('browser', 'chromium');
const root = mkdtempSync(join(tmpdir(), 'dq-replay-guard-'));
try {
  const clip = join(root, 'incorrect.mp4');
  const poster = join(root, 'incorrect.png');
  const run = spawnSync(process.execPath, [
    fileURLToPath(new URL('./render-preview.mjs', import.meta.url)),
    '--base', base, '--game', 'penguin-heartlight-rescue',
    '--run', 'nonexistent-preview-replay-contract-id',
    '--out', clip, '--poster', poster, '--browser', browser,
    '--duration-ms', '1000', '--fps', '4',
  ], { encoding: 'utf8', timeout: 30000 });
  if (run.error) throw run.error;
  assert.notEqual(run.status, 0, 'missing replay must not produce a successful clip');
  assert.match(run.stderr, /recorded replay unavailable: HTTP 404/,
    'failure should distinguish an absent recording from successful capture');
  assert.equal(existsSync(clip), false, 'no clip may be attributed to a nonexistent run');
  assert.equal(existsSync(poster), false, 'no poster may be attributed to a nonexistent run');
  console.log(JSON.stringify({ passed: true, browser, missingRunRejected: true,
    noMisattributedClip: true, noMisattributedPoster: true }));
} finally {
  rmSync(root, { recursive: true, force: true });
}
