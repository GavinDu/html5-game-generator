import test from 'node:test';
import assert from 'node:assert/strict';
import { resolveHorizontalMovement } from '../../internal/api/web/runtime-movement-plane.mjs';

const old = (scheme, archetype = 'catcher', objective = 'hearts_collected') => ({
  irVersion: '1.2',
  capabilities: ['shell.v1'],
  experience: {
    archetype,
    controls: { scheme },
    objective: { kind: 'collect', progressVariable: objective },
  },
});
const modern = (axes, scheme = 'tap', objective = 'hearts_collected') => ({
  ...old(scheme, 'catcher', objective),
  capabilities: ['shell.v1', 'input.movement_axes.v1'],
  experience: {
    ...old(scheme, 'catcher', objective).experience,
    controls: { scheme, movementAxes: axes },
  },
});

test('legacy published catcher games retain exact historical movement despite adapter extraction', () => {
  assert.equal(resolveHorizontalMovement(old('tap', 'catcher', 'hearts_collected')), true, 'Penguin');
  assert.equal(resolveHorizontalMovement(old('tap', 'catcher', 'berries_collected')), true, 'Bobo');
  assert.equal(resolveHorizontalMovement(old('tap', 'catcher', 'holes_stitched')), false, 'Ribbon');
  assert.equal(resolveHorizontalMovement(old('drag_x', 'catcher', 'holes_stitched')), true);
  assert.equal(resolveHorizontalMovement(old('drag_free', 'catcher', 'hearts_collected')), false);
  assert.equal(resolveHorizontalMovement(old('lane_tap', 'dodger', 'gap')), true);
  assert.equal(resolveHorizontalMovement({ irVersion: '1.0' }), false);
});

test('versioned x/xy movement is independent of the game objective or archetype identity', () => {
  assert.equal(resolveHorizontalMovement(modern('xy', 'tap', 'hearts_collected')), false);
  assert.equal(resolveHorizontalMovement(modern('x', 'tap', 'holes_stitched')), true);
  assert.equal(resolveHorizontalMovement(modern('x', 'tap', 'hearts_collected')), true);
  assert.equal(resolveHorizontalMovement(modern('xy', 'tap', 'holes_stitched')), false);
  assert.equal(resolveHorizontalMovement(modern('x', 'lane_tap')), true);
  assert.equal(resolveHorizontalMovement(modern('xy', 'drag_free')), false);
});

test('adapter fails closed for unversioned, unsupported and contradictory movement contracts', () => {
  const invalid = [
    { ...modern('xy'), irVersion: '1.1' },
    modern('z'),
    modern(''),
    modern('xy', 'drag_x'),
    modern('x', 'drag_free'),
    modern('xy', 'lane_tap'),
    modern('x', 'tap_target'),
    { ...old('tap'), experience: { ...old('tap').experience, controls: { scheme: 'tap', movementAxes: 'xy' } } },
  ];
  for (const ir of invalid) {
    assert.throws(() => resolveHorizontalMovement(ir),
      Error, 'invalid or undeclared axis contract must be rejected: ' + JSON.stringify(ir));
  }
});
