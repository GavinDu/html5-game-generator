import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const sourcePath = fileURLToPath(new URL('./render-preview.mjs', import.meta.url));
const source = await readFile(sourcePath, 'utf8');
const runtimePath = fileURLToPath(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url));
const runtime = await readFile(runtimePath, 'utf8');

test('preview renderer persists the first usable poster before capturing later frames', () => {
  assert.match(source, /if \(!posterReady\)/);
  assert.match(source, /writeFileSync\(poster, await .*readFile\(framePath\)\)/);
  assert.match(source, /posterReady = true/);
});

test('render failures after poster capture produce poster-only output and remove partial clips', () => {
  assert.match(source, /render failed after poster capture; keeping poster/);
  assert.match(source, /clip: null, clip_bytes: 0, poster/);
  assert.match(source, /rmSync\(out, \{ force: true \}\)/);
  assert.match(source, /fallback_reason:/);
});

test('preview renderer reports per-game media metrics', () => {
  assert.match(source, /clip_bytes: clipBytes/);
  assert.match(source, /poster_bytes: posterBytes/);
  assert.match(source, /render_ms:/);
});


test('preview capture advances the runtime to exact fixed simulation steps', () => {
  assert.match(source, /probe=1&capture=1&replay=/);
  assert.match(source, /advanceToStep\(step\)/);
  assert.match(source, /snapshot\??\.simStep === step/);
  assert.doesNotMatch(source, /waitForTimeout\(frameInterval\)/);
  assert.match(runtime, /const deterministicCapture = probeMode && params\.get\('capture'\) === '1'/);
  assert.match(runtime, /advanceToStep: \(step\) =>/);
  assert.match(runtime, /capturePaused/);
  assert.match(runtime, /play\.simStep < captureTargetStep/);
  assert.match(runtime, /if \(deterministicCapture\) \{\s*try \{ app\.renderer\.render\(app\.stage\);/);
});
