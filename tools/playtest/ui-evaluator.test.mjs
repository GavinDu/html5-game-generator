import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const source = await readFile(fileURLToPath(new URL('./ui-evaluator.mjs', import.meta.url)), 'utf8');
const runtimeSource = await readFile(fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url)), 'utf8');

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


test('UI evaluator fails when no shell controls are measured instead of passing an empty set', () => {
  assert.match(source, /shell-controls-present/);
  assert.match(source, /metrics\.shellButtons\.length > 0/);
  assert.match(source, /metrics\.shellButtons\.length > 0 && metrics\.shellButtons\.every/);
});


test('UI evaluator emits a weighted per-viewport score and overall grade', () => {
  assert.match(source, /const weights =/);
  assert.match(source, /const score = gameplayProfile \? checks\.reduce/);
  assert.match(source, /score: averageScore/);
  assert.match(source, /grade: averageScore >= 90/);
  assert.match(source, /results\.length === viewports\.length/);
});


test('UI evaluator accepts a full-canvas touch surface only when the IR declares touch controls', () => {
  assert.match(runtimeSource, /controlsScheme: play\.shell\.controls\?\.scheme/);
  assert.match(source, /const shellControlScheme = window\.__dqProbe\?\.shell\?\.controlsScheme/);
  assert.match(source, /const declaredTouchControls = \['tap', 'tap_target', 'drag_x', 'drag_free'\]\.includes\(shellControlScheme\)/);
  assert.match(source, /const canvasIsTouchSurface = !!rect &&\s+declaredTouchControls/);
  assert.match(source, /canvasIsTouchSurface/);
  assert.match(source, /full-canvas /);
});


test('unsupported orientation is evaluated only as a paused fallback', () => {
  assert.match(source, /supportsViewport\(orientation, viewport.width, viewport.height\)/);
  assert.match(source, /mode: gameplayProfile \? "gameplay" : "rotation-fallback"/);
  assert.match(source, /orientation-guidance-and-pause/);
  assert.match(source, /results.filter\(result => result.mode === 'gameplay'\)/);
});
