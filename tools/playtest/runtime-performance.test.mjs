import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtimeURL = new URL('../../internal/api/web/pixi-runtime.js', import.meta.url);
const source = await readFile(runtimeURL, 'utf8');

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
test('drag_x shell controls keep pointer and keyboard movement on the horizontal axis', () => {
  assert.match(source, /const dragXOnly = ir\.experience\?\.controls\?\.scheme === 'drag_x'/);
  assert.match(source, /pointer\.y = dragXOnly && playerEnt \? playerTarget\.y : p\.y/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_up'\)\)/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_down'\)\)/);
  assert.match(source, /if \(!dragXOnly\) playerTarget\.y = pointer\.y/);
});
