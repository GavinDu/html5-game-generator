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

test('all semantic updates run at fixed 60Hz independently of display frames', () => {
  assert.match(source, /const STEP_MS = 1000 \/ 60/);
  assert.match(source, /tickFrame\(STEP_MS\)/);
  assert.match(source, /play\.simStep\+\+/);
  assert.match(source, /world\.step\(eventQueue\)/);
});

test('one-shot pickup overlap recovery is game-independent and idempotent', () => {
  assert.doesNotMatch(source, /progressVariable === 'holes_stitched'/);
  assert.match(source, /collectible\.def\.properties\?\.closeOnCollect/);
  assert.match(source, /collectible\.def\.kind === 'pickup'/);
  assert.match(source, /handleContact\(playerEnt, collectible, true\)/);
  assert.match(source, /if \(!state\.collectedPairs\.has\(key\)\)/);
  assert.match(source, /if \(other\.destroyed \|\| other\.closed\) return/);
});
