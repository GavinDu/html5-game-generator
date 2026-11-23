import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtimeURL = new URL('../../internal/api/web/pixi-runtime.js', import.meta.url);
const source = await readFile(runtimeURL, 'utf8');
const clockSource = await readFile(new URL('../../internal/api/web/runtime-fixed-step-clock.mjs', import.meta.url), 'utf8');
const probeURL = new URL('./probe.mjs', import.meta.url);
const probeSource = await readFile(probeURL, 'utf8');

test('runtime image loading is bounded and concurrent', () => {
  assert.match(source, /const imageWorkerCount = Math\.min\(6, imageAssets\.length\)/);
  assert.match(source, /await awaitActive\(Promise\.all\(Array\.from\(\{ length: imageWorkerCount \}/);
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
  // The hold must still last 220ms across animation frames, but now
  // the record is contingent on both real dispatch calls succeeding.
  assert.match(probeSource, /\(\) => sleep\(220\)/);
  assert.match(probeSource, /recordAfterGesture\(\[/);
  assert.match(probeSource, /\(\) => recordInput\('synthetic_pointer_hold'/);
});

test('drag_x shell controls keep pointer and keyboard movement on the horizontal axis', () => {
  assert.match(source, /const dragXOnly = resolveHorizontalMovement\(ir\)/);
  assert.match(source, /from '\.\/runtime-movement-plane\.mjs'/);
  assert.doesNotMatch(source, /holes_stitched/);
  assert.match(source, /pointer\.y = dragXOnly && playerEnt \? playerTarget\.y : p\.y/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_up'\)\)/);
  assert.match(source, /if \(!dragXOnly && keysDown\.has\('move_down'\)\)/);
  assert.match(source, /if \(!dragXOnly\) playerTarget\.y = pointer\.y/);
});

test('discrete lane taps clamp to three x centers and never follow held pointer y', () => {
  assert.match(source, /const laneTap = ir\.experience\?\.controls\?\.scheme === 'lane_tap'/);
  assert.match(source, /const laneCenters = \[W \* 0\.25, W \* 0\.5, W \* 0\.75\]/);
  assert.match(source, /function beginPointer\(x, y\)/);
  assert.match(source, /laneIndex = clamp\(laneIndex \+ \(x < W \/ 2 \? -1 : 1\), 0, 2\)/);
  assert.match(source, /beginPointer\(p\.x, p\.y\)/);
  assert.match(source, /beginPointer\(x, y\)/);
  assert.match(source, /if \(pointer\.down && !laneTap\)/);
  assert.match(source, /playerTarget\.y = playerEnt\.anchorY/);
});

test('all semantic updates run at fixed 60Hz independently of display frames', () => {
  assert.match(source, /const simClock = createFixedStepClock\(\)/);
  assert.match(source, /const STEP_MS = simClock\.stepMS/);
  assert.match(clockSource, /export const FIXED_STEP_MS = 1000 \/ 60;/);
  assert.match(clockSource, /export const MAX_CATCHUP_STEPS = 3;/);
  assert.match(source, /simClock\.finishFrame\(steps\)/);
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

test('render submission is reported before the unique runtime player_ready event', () => {
  const index = source.indexOf('const firstSceneRenderSubmitMs =');
  assert.ok(index > 0, 'missing first render timestamp');
  const rendered = source.lastIndexOf('app.renderer.render(app.stage);', index);
  assert.ok(rendered >= 0 && rendered < index, 'timestamp cannot precede renderer submission');
  assert.match(source, /sendTelemetryEvent\('milestone', \{ milestone: 'first_scene_render_submit'/);
  assert.match(source, /sendNativeMetric\('first_scene_render_submit', \{ ms: firstSceneRenderSubmitMs \}\)/);
  const firstReady = source.indexOf("sendSocialDiagnostic('player_ready', 'ready'");
  assert.ok(firstReady > index, 'readiness may not precede first render');
  assert.equal(source.indexOf("sendSocialDiagnostic('player_ready', 'ready'", firstReady + 1), -1,
    'duplicate runtime readiness report');
});

test('terminal and failed state block late rule dispatch, execution and collision contacts', () => {
  // A budget failure must be as terminal as a legitimate result: no
  // subsequent contact may restart work or mutate gameplay state.
  assert.match(source, /function emit\(event, source, target, value, inputAction\) \{[^]*?if \(state\.ended \|\| state\.failed\) return;/);
  assert.match(source, /function execute\(actions, sourceEnt, targetEnt, startupRule\) \{\s*if \(state\.ended \|\| state\.failed\) return;/);
  assert.match(source, /function handleContact\(a, b, started\) \{\s*if \(state\.ended \|\| state\.failed \|\| !playerEnt\) return;/);
});


test('image workers enforce streaming byte ceilings and retain a bounded fetch deadline', () => {
  assert.match(source, /const MAX_IMAGE_TRANSFER_BYTES = 8 \* 1024 \* 1024/);
  assert.match(source, /const reader = response\.body\.getReader\(\)/);
  assert.match(source, /if \(totalBytes > MAX_IMAGE_TRANSFER_BYTES\)/);
  assert.match(source, /await reader\.cancel\(\)\.catch/);
  assert.match(source, /blob\.size > MAX_IMAGE_TRANSFER_BYTES/);
  const authenticatedFetch = source.indexOf('authenticatedAssetFetchOptions(requestController.signal)');
  const bodyRead = source.indexOf('imageBlob = await readBoundedImageBlob(response)');
  const stopTimeout = source.indexOf('clearTimeout(requestTimeout)', bodyRead);
  assert.ok(authenticatedFetch > 0 && authenticatedFetch < bodyRead,
    'private image transport must preserve the current authorization flow');
  assert.ok(bodyRead > authenticatedFetch && stopTimeout > bodyRead,
    'abort deadline must remain active until the response body is consumed');
});

test('decoded image dimensions and aggregate pixel quota are checked before GPU upload', () => {
  assert.match(source, /const MAX_IMAGE_DIMENSION = 4096/);
  assert.match(source, /const MAX_IMAGE_PIXELS = 16 \* 1024 \* 1024/);
  assert.match(source, /const MAX_TOTAL_IMAGE_PIXELS = 32 \* 1024 \* 1024/);
  assert.match(source, /pixels > MAX_IMAGE_PIXELS/);
  assert.match(source, /decodedImagePixels \+ pixels > MAX_TOTAL_IMAGE_PIXELS/);
  const bounds = source.indexOf('decodedImagePixels + pixels > MAX_TOTAL_IMAGE_PIXELS');
  const activeGate = source.indexOf('attempt.assertActive();', bounds);
  const upload = source.indexOf('const texture = PIXI.Texture.from(img)', bounds);
  assert.ok(bounds >= 0 && activeGate > bounds && upload > activeGate,
    'current game run must remain active and texture size must be validated before GPU upload');
});
