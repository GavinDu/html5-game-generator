import test from 'node:test';
import assert from 'node:assert/strict';
import { makeCaughtFixture } from '../terminal-fixture.mjs';

function sample() {
  return { ir: {
    experience: { controls: { scheme: 'lane_tap' },
      chaserGap: { start: 220, max: 250, caughtAt: 55, recoveryPerSecond: 6 } },
    scenes: [{ entities: [
      { id: 'player', position: [195, 660] },
      { id: 'syringe', position: [110, 240], behavior: { type: 'chase' },
        physics: { body: 'kinematic_velocity', shape: 'capsule' } },
    ] }],
    rules: [{ when: { event: 'collision', target: 'syringe' },
      actions: [{ type: 'add_variable', variable: 'chaser_gap', value: -45 }] }]
  } };
}

test('controlled terminal fixture preserves canonical package and uses actual collider', () => {
  const original = sample();
  const fixture = makeCaughtFixture(original);
  assert.equal(fixture.ir.experience.chaserGap.start, 70);
  assert.deepEqual(fixture.ir.scenes[0].entities[1].position, [195, 660]);
  assert.equal(fixture.ir.scenes[0].entities[1].physics.body, 'dynamic');
  assert.equal(fixture.ir.scenes[0].entities[1].physics.sensor, true);
  assert.equal(fixture.ir.scenes[0].entities[1].behavior, undefined);
  assert.equal(original.ir.experience.chaserGap.start, 220);
  assert.deepEqual(original.ir.scenes[0].entities[1].position, [110, 240]);
});

test('fixture fails closed without valid authored negative-gap collision', () => {
  const noCollision = sample();
  noCollision.ir.rules = [];
  assert.equal(makeCaughtFixture(noCollision), null);
  const noLane = sample();
  noLane.ir.experience.controls.scheme = 'drag_free';
  assert.equal(makeCaughtFixture(noLane), null);
});
