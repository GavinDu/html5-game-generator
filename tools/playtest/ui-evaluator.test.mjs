import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const source = await readFile(fileURLToPath(new URL('./ui-evaluator.mjs', import.meta.url)), 'utf8');

test('UI evaluator covers iPhone and iPad in portrait and landscape', () => {
  for (const name of ['iphone-portrait', 'iphone-landscape', 'ipad-portrait', 'ipad-landscape']) {
    assert.match(source, new RegExp(name));
  }
});

test('UI evaluator fails closed on overflow, clipped canvas, unrendered canvas, and page errors', () => {
  for (const check of ['no-horizontal-overflow', 'no-vertical-overflow', 'canvas-visible', 'canvas-rendered', 'no-page-errors']) {
    assert.match(source, new RegExp(check));
  }
  assert.match(source, /if \(!report\.passed\) process\.exitCode = 1/);
});

test('UI evaluator records screenshots and checks shell touch target sizing', () => {
  assert.match(source, /page\.screenshot\(/);
  assert.match(source, /shell-controls-touch-sized/);
  assert.match(source, /button\.width >= 44 && button\.height >= 44/);
});
