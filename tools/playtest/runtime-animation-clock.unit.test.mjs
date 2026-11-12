import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtime = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('Pixi animated sprites never register with the display ticker for gameplay', () => {
  assert.match(runtime, /new PIXI\.AnimatedSprite\(frameNames\.map\(\(f\) => textures\.get\(f\)\)\);\s*\/\/[^]*?view\.autoUpdate = false;/);
  assert.match(runtime, /view\.animationSpeed = \(r\.animation\.fps \?\? 8\) \/ 60;/);
  assert.match(runtime, /view\.onComplete = \(\) => emit\('animation_complete', ent, ent\)/);
});

test('only the fixed-step semantic entity loop advances AnimatedSprite frames', () => {
  assert.match(runtime, /function tickFrame\(dtMs\) \{/);
  assert.match(runtime, /if \(ent\.view instanceof PIXI\.AnimatedSprite\) \{\s*ent\.view\.update\(\{ deltaTime: dtMs \/ \(1000 \/ 60\), deltaMS: dtMs \}\);/);
  assert.doesNotMatch(runtime, /Ticker\.shared\.add\(/);
  assert.doesNotMatch(runtime, /app\.ticker\.add\([^]*?AnimatedSprite\.update/);
  assert.match(runtime, /animations: \[\.\.\.entities\.values\(\)\]/);
});
