import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./verify-preview-determinism.mjs', import.meta.url), 'utf8');

test('verifier always renders same recorded run twice and checks frame hashes', () => {
  assert.ok(source.includes("render('first')"));
  assert.ok(source.includes("render('second')"));
  assert.ok(source.includes("'--run', run"));
  assert.ok(source.includes("compareFrameHashes(first.frame_sha256, second.frame_sha256)"));
  assert.ok(source.includes("if (!comparison.match) process.exitCode = 1"));
});

test('verifier rejects fallback, missing frame hashes and removes temp output', () => {
  assert.ok(source.includes('!report.clip || report.clip_fallback'));
  assert.ok(source.includes('!Array.isArray(report.frame_sha256)'));
  assert.ok(source.includes("rmSync(temp, { recursive: true, force: true })"));
});

test('native aspect selection requires paired explicit viewport overrides', () => {
  assert.ok(source.includes("const width = arg('width');"));
  assert.ok(source.includes("const height = arg('height');"));
  assert.ok(source.includes("if (width) renderArgs.push('--width', width, '--height', height)"));
  assert.ok(!source.includes("const width = arg('width', '420')"));
  assert.ok(source.includes("width: first.width, height: first.height"));
  assert.ok(source.includes("aspect: first.aspect, orientation: first.orientation"));
});

test('invalid one-sided viewport arguments fail before creating temp captures', () => {
  const guard=source.indexOf('if ((!!width !== !!height)');
  const temp=source.indexOf("mkdtempSync(join(tmpdir()");
  assert.ok(guard >= 0 && temp > guard);
  assert.ok(source.includes('Number.isInteger(Number(width))'));
  assert.ok(source.includes('Number.isInteger(Number(height))'));
  assert.ok(source.includes('provide both --width and --height'));
  assert.ok(source.includes('process.exit(2)'));
});

test('preview replay determinism forwards and checks the same simulated start offset', () => {
  assert.ok(source.includes("const captureStartMs = arg('capture-start-ms', '0')"));
  assert.ok(source.includes("'--capture-start-ms', captureStartMs"));
  assert.ok(source.includes("first.capture_start_step !== second.capture_start_step"));
  assert.ok(source.includes("capture_start_ms: first.capture_start_ms"));
  assert.ok(source.includes("capture_start_step: first.capture_start_step"));
});
