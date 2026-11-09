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


test('preview renderer rejects non-finite numeric options instead of emitting an empty successful preview', () => {
  assert.match(source, /!Number\.isFinite\(durationMs\) \|\| !Number\.isFinite\(fps\) \|\| !Number\.isFinite\(startDelayMs\)/);
  assert.match(source, /width and height must be integers from 240 to 4096/);
});


test('preview renderer records a SHA-256 digest for each captured frame', () => {
  assert.match(source, /import \{ createHash \} from 'node:crypto'/);
  assert.match(source, /frameHashes\.push\(createHash\('sha256'\)\.update\(frame\)\.digest\('hex'\)\)/);
  assert.match(source, /frame_sha256: frameHashes/);
  assert.match(source, /frame_sha256: frameHashes, fps/);
});

 
test('preview renderer supports bounded native portrait, landscape, and tablet viewports', () => {
  assert.match(source, /let width = Number\(arg\('width', '420'\)\)/);
  assert.match(source, /let height = Number\(arg\('height', '900'\)\)/);
  assert.match(source, /viewport: \{ width, height \}/);
  assert.match(source, /orientation: width === height \? 'square' : width > height \? 'landscape' : 'portrait'/);
  assert.match(source, /width < 240 \|\| height < 240 \|\| width > 4096 \|\| height > 4096/);
});

test('preview rendering refuses missing or stale recorded runs before capturing pixels', () => {
  assert.match(source, /validateRecordedRun\(await runResponse\.json\(\)/);
  assert.match(source, /recorded replay unavailable: HTTP/);
  assert.match(source, /sourceRun\.id/);
  assert.match(source, /capture runtime did not load the requested recorded input trace/);
  assert.match(runtime, /deterministic capture requires the exact recorded run/);
  assert.match(runtime, /recorded replay IR hash differs from the current package/);
});
