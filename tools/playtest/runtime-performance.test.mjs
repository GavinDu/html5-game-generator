import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtimeURL = new URL('../../internal/api/web/pixi-runtime.js', import.meta.url);
const source = await readFile(runtimeURL, 'utf8');
const probeURL = new URL('./probe.mjs', import.meta.url);
const probeSource = await readFile(probeURL, 'utf8');

test('runtime image loading is bounded and concurrent', () => {
  assert.match(source, /const imageWorkerCount = Math\.min\(6, imageAssets\.length\)/);
  assert.match(source, /await Promise\.all\(Array\.from\(\{ length: imageWorkerCount \}/);
  assert.doesNotMatch(source, /for \(const a of pkg\.assets \|\| \[\]\) \{\s*if \(a\.kind !== 'image'\) continue;\s*try \{\s*textures\.set\(a\.id, await loadTexture/);
});

test('a stalled image request and image decode have finite timeouts', () => {
  assert.match(source, /image request timed out after 15s/);
  assert.match(source, /image decode timed out after 15s/);
  assert.match(source, /clearTimeout\(requestTimeout\)/);
  assert.match(source, /clearTimeout\(timeout\)/);
});

test('visible developer logs stay bounded like the backing buffer', () => {
  assert.match(source, /if \(logBuffer\.length > 50\) logBuffer\.shift\(\)/);
  assert.ok(source.includes("logEl.textContent = logBuffer.join('\\n') + '\\n';"));
  assert.ok(!source.includes("logEl.textContent += line"));
});
test('custom collect progress is independent from score and increments through rules', () => {
  assert.match(source, /function objectiveProgress\(\)/);
  assert.match(source, /progressVariable \? Number\(state\.variables\[progressVariable\] \?\? 0\) : state\.score/);
  assert.match(source, /case 'add_variable'/);
  assert.match(source, /state\.variables\[action\.variable\] = current \+ delta/);
  assert.match(source, /emit\('variable_changed', sourceEnt, targetEnt\)/);
  assert.match(source, /objectiveProgress\(\).*of \$\{target\}/s);
});

test('tap catcher falls back to a held pointer when touch is recorded without movement', () => {
  assert.match(probeSource, /const needsHeldContact = afterInputs <= beforeInputs \|\| !points \|\| \(targetDelta > 12 && moved < 4\)/);
  assert.match(probeSource, /if \(needsHeldContact\)/);
  assert.match(probeSource, /await sleep\(220\)/);
});

test('drag_x shell controls keep pointer and keyboard movement on the horizontal axis', () => {
  assert.match(source, /const dragXOnly = laneTap \|\| ir\.experience\?\.controls\?\.scheme === 'drag_x' \|\|/);
  assert.match(source, /ir\.experience\?\.archetype === 'catcher' && ir\.experience\?\.controls\?\.scheme === 'tap'/);
  assert.match(source, /pointer\.y = dragXOnly && playerEnt \? playerTarget\.y : p\.y/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_up'\)\)/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_down'\)\)/);
  assert.match(source, /if \(!dragXOnly\) playerTarget\.y = pointer\.y/);
});

test('discrete lane taps clamp to three x centers and never follow held pointer y', () => {
  assert.match(source, /const laneTap = ir\.experience\?\.controls\?\.scheme === 'lane_tap'/);
  assert.match(source, /const laneCenters = \[W \* 0\.25, W \* 0\.5, W \* 0\.75\]/);
  assert.match(source, /laneIndex = clamp\(laneIndex \+ \(p\.x < W \/ 2 \? -1 : 1\), 0, 2\)/);
  assert.match(source, /if \(pointer\.down && !laneTap\)/);
  assert.match(source, /playerTarget\.y = playerEnt\.anchorY/);
});

test('image workers cap bytes while streaming and keep fetch timeout until download finishes', () => {
  assert.match(source, /const MAX_IMAGE_TRANSFER_BYTES = 8 \* 1024 \* 1024/);
  assert.match(source, /const reader = response\.body\.getReader\(\)/);
  assert.match(source, /if \(totalBytes > MAX_IMAGE_TRANSFER_BYTES\)/);
  assert.match(source, /await reader\.cancel\(\)\.catch/);
  const fetch = source.indexOf('imageBlob = await readBoundedImageBlob(response)');
  const timeout = source.indexOf('clearTimeout(requestTimeout)', fetch);
  assert.ok(fetch > 0 && timeout > fetch, 'download must remain under 15s abort deadline');
});

test('decoded texture dimensions and aggregate pixel count are bounded before Pixi upload', () => {
  assert.match(source, /const MAX_IMAGE_DIMENSION = 4096/);
  assert.match(source, /const MAX_TOTAL_IMAGE_PIXELS = 32 \* 1024 \* 1024/);
  assert.match(source, /pixels > MAX_IMAGE_PIXELS/);
  assert.match(source, /decodedImagePixels \+ pixels > MAX_TOTAL_IMAGE_PIXELS/);
  const bounds = source.indexOf('decodedImagePixels + pixels > MAX_TOTAL_IMAGE_PIXELS');
  const upload = source.indexOf('const texture = PIXI.Texture.from(img)', bounds);
  assert.ok(bounds >= 0 && upload > bounds, 'validate before creating GPU texture');
});
