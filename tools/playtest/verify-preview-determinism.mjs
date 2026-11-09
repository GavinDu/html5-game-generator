#!/usr/bin/env node
// Render the same recorded run twice and fail unless every captured frame is
// byte-identical. This is an opt-in end-to-end check for issue #100.
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { compareFrameHashes } from './frame-hash-comparison.mjs';
import { MAX_CAPTURE_START_MS } from './capture-window.mjs';

function arg(name, fallback = '') {
  const index = process.argv.indexOf(`--${name}`);
  return index >= 0 && process.argv[index + 1] ? process.argv[index + 1] : fallback;
}

const base = arg('base').replace(/\/$/, '');
const game = arg('game');
const run = arg('run');
const browser = arg('browser', 'chromium');
const durationMs = arg('duration-ms', '6000');
const fps = arg('fps', '12');
const captureStartMs = arg('capture-start-ms', '0');
// Omitting both dimensions lets render-preview select the declared native
// game orientation. A portrait-only default used to reject landscape runs.
const width = arg('width');
const height = arg('height');
if (!base || !game || !run) {
  console.error('usage: node verify-preview-determinism.mjs --base URL --game ID --run RUN_ID [--browser chromium|webkit]');
  process.exit(2);
}
if (!['chromium', 'webkit'].includes(browser)) {
  console.error('verify-preview-determinism.mjs: --browser must be chromium or webkit');
  process.exit(2);
}

if ((!!width !== !!height) ||
    (width && (!Number.isInteger(Number(width)) || !Number.isInteger(Number(height)) ||
      Number(width) < 240 || Number(height) < 240 ||
      Number(width) > 4096 || Number(height) > 4096))) {
  console.error('verify-preview-determinism: provide both --width and --height in [240,4096] or neither');
  process.exit(2);
}

// Both replay renders must select the same approved simulated gameplay window.
// Wall-clock waits cannot stand in for advancing a deterministic trace.
if (!Number.isSafeInteger(Number(captureStartMs)) ||
    Number(captureStartMs) < 0 || Number(captureStartMs) > MAX_CAPTURE_START_MS) {
  console.error('verify-preview-determinism: --capture-start-ms must be an integer in [0,30000]');
  process.exit(2);
}

const temp = mkdtempSync(join(tmpdir(), 'dq-preview-determinism-'));
const renderer = fileURLToPath(new URL('./render-preview.mjs', import.meta.url));
function render(label) {
  const out = join(temp, label + '.mp4');
  const poster = join(temp, label + '.png');
  const renderArgs = [
    renderer, '--base', base, '--game', game, '--run', run, '--out', out,
    '--poster', poster, '--browser', browser, '--duration-ms', durationMs, '--fps', fps,
    '--capture-start-ms', captureStartMs,
  ];
  if (width) renderArgs.push('--width', width, '--height', height);
  const result = spawnSync(process.execPath, renderArgs,
    { encoding: 'utf8', timeout: 180000, maxBuffer: 8 * 1024 * 1024 });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${label} render failed (exit ${result.status}): ${result.stderr.trim()}`);
  const line = result.stdout.trim().split(/\r?\n/).at(-1);
  const report = JSON.parse(line);
  if (!report.clip || report.clip_fallback || report.frames < 1 || !Array.isArray(report.frame_sha256)) {
    throw new Error(`${label} did not produce a complete clip with frame hashes`);
  }
  return report;
}

try {
  const first = render('first');
  const second = render('second');
  if (first.capture_start_step !== second.capture_start_step ||
      first.capture_start_ms !== second.capture_start_ms ||
      first.capture_start_ms !== Number(captureStartMs)) {
    throw new Error('recorded preview capture offsets differed between renders');
  }
  const comparison = compareFrameHashes(first.frame_sha256, second.frame_sha256);
  const result = {
    game, run, browser, capture_start_ms: first.capture_start_ms,
    capture_start_step: first.capture_start_step,
    width: first.width, height: first.height,
    aspect: first.aspect, orientation: first.orientation, deterministic: comparison.match,
    frame_count: comparison.frame_count,
    first_mismatch_index: comparison.first_mismatch_index,
    first_clip_bytes: first.clip_bytes,
    second_clip_bytes: second.clip_bytes,
  };
  console.log(JSON.stringify(result));
  if (!comparison.match) process.exitCode = 1;
} catch (error) {
  console.error(`verify-preview-determinism.mjs: ${error.message}`);
  process.exitCode = 1;
} finally {
  rmSync(temp, { recursive: true, force: true });
}
