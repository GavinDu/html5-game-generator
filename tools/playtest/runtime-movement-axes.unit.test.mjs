import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const runtime = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
const goSchema = await readFile(new URL('../../internal/gameir/experience.go', import.meta.url), 'utf8');
const goCaps = await readFile(new URL('../../internal/gameir/ir.go', import.meta.url), 'utf8');
const swiftContract = await readFile(
  new URL('../../ios/GamePlayerCore/Sources/GamePlayerCore/PlayerLifecycle.swift', import.meta.url), 'utf8');

test('Go, browser and Swift contract negotiate exactly the same versioned movement capability', () => {
  const cap = 'input.movement_axes.v1';
  assert.ok(runtime.includes(cap));
  assert.ok(goSchema.includes(cap));
  assert.ok(goCaps.includes(cap));
  assert.ok(swiftContract.includes(cap));
  assert.match(goSchema, /MovementAxes string `json:"movementAxes,omitempty"`/);
});

test('browser validates movement-plane contract before allocating a game scene', () => {
  const capabilityCheck = runtime.indexOf("checkedCapabilities.has('input.movement_axes.v1')");
  const sceneAllocation = runtime.indexOf('const scene = (ir.scenes');
  assert.ok(capabilityCheck > 0 && capabilityCheck < sceneAllocation);
  assert.match(runtime, /if \(ir.irVersion !== '1.2' \|\| !\['x', 'xy'\]\.includes\(explicitMovementAxes\)\)/);
  assert.match(runtime, /throw new Error\('movementAxes requires input\.movement_axes\.v1 capability'\)/);
});

test('new declarative movement axes override objective while legacy IR stays compatible', () => {
  assert.match(runtime, /const dragXOnly = resolveHorizontalMovement\(ir\)/);
  assert.match(runtime, /from '\.\/runtime-movement-plane\.mjs'/);
  assert.doesNotMatch(runtime, /holes_stitched/);
  assert.match(runtime, /if \(!dragXOnly\) playerTarget\.y = p\.y/);
  assert.match(runtime, /if \(!dragXOnly && keysDown\.has\('move_up'\)\)/);
});
