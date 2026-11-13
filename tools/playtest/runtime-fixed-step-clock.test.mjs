import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  createFixedStepClock, FIXED_STEP_MS, MAX_CATCHUP_STEPS, MAX_FRAME_DELTA_MS,
} from '../../internal/api/web/runtime-fixed-step-clock.mjs';

// Frozen exact 2026-11-12 production accumulator, including floating-point
// evaluation order and overload shedding. No "improved" time correction:
// even a small change alters tick-stamped input and published replay results.
function historical(deltas, stopOnTick = Infinity) {
  let accumulatorMS = 0;
  let totalTicks = 0;
  const ticksPerFrame = [];
  for (const deltaMS of deltas) {
    accumulatorMS += Math.min(deltaMS, 250);
    let steps = 0;
    while (accumulatorMS >= 1000 / 60 && steps < 3) {
      totalTicks++;
      accumulatorMS -= 1000 / 60;
      steps++;
      if (totalTicks === stopOnTick) break;
    }
    if (steps === 3 && accumulatorMS >= 1000 / 60) accumulatorMS = 0;
    ticksPerFrame.push(steps);
    if (totalTicks === stopOnTick) break;
  }
  return { totalTicks, ticksPerFrame, accumulatorMS };
}

function modular(deltas, stopOnTick = Infinity) {
  const clock = createFixedStepClock();
  let totalTicks = 0;
  const ticksPerFrame = [];
  for (const deltaMS of deltas) {
    clock.addFrame(deltaMS);
    let steps = 0;
    while (clock.hasPendingStep(steps)) {
      totalTicks++;
      clock.completedStep();
      steps++;
      if (totalTicks === stopOnTick) break;
    }
    clock.finishFrame(steps);
    ticksPerFrame.push(steps);
    if (totalTicks === stopOnTick) break;
  }
  return { totalTicks, ticksPerFrame, accumulatorMS: clock.snapshot().accumulatorMS };
}

test('exact legacy policy constants and production interpreter import', () => {
  assert.equal(FIXED_STEP_MS, 1000 / 60);
  assert.equal(MAX_CATCHUP_STEPS, 3);
  assert.equal(MAX_FRAME_DELTA_MS, 250);
  const runtime = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
  assert.match(runtime, /import \{ createFixedStepClock \} from '\.\/runtime-fixed-step-clock\.mjs'/);
  assert.match(runtime, /const simClock = createFixedStepClock\(\)/);
  assert.match(runtime, /simClock\.addFrame\(ticker\.deltaMS\)/);
  assert.match(runtime, /while \(simClock\.hasPendingStep\(steps\)\)/);
  assert.match(runtime, /simClock\.completedStep\(\)/);
  assert.match(runtime, /simClock\.finishFrame\(steps\)/);
});

test('modular frame clock bit-for-bit reproduces old cadence under 30, 60, 120 Hz', () => {
  for (const fps of [30, 60, 120]) {
    const deltas = Array.from({ length: fps * 8 }, () => 1000 / fps);
    assert.deepEqual(modular(deltas), historical(deltas), 'fps=' + fps);
    assert.equal(modular(deltas).totalTicks, 480, 'eight sim-seconds at fps=' + fps);
  }
});

test('jitter and overload shed simulation backlog identically to shipped games', () => {
  const samples = [0, 1, 1000 / 120, 1000 / 30, 20, 19, 50, 100, 250, 1000,
    0, 500, 80, 1, 250, 15, 15, 15, 0, 17, 1000 / 60];
  const deltas = [...samples, ...samples, ...samples, ...samples];
  assert.deepEqual(modular(deltas), historical(deltas));
  assert.equal(modular([100]).totalTicks, 3, '100 ms display stall may not run 6 sim steps');
  assert.equal(modular([100]).accumulatorMS, 0, 'overload backlog is intentionally discarded');
  assert.equal(modular([250]).totalTicks, 3, 'per-frame delta is clamped');
  assert.equal(modular([1000]).totalTicks, 3, 'one-second stall is bounded by three steps');
});

test('terminal condition exits after one semantic tick without consuming extra steps', () => {
  const deltas = [15, 20, 50, 100, 33, 250, 16];
  for (const stopOnTick of [1, 2, 3, 5]) {
    assert.deepEqual(modular(deltas, stopOnTick), historical(deltas, stopOnTick));
  }
});

test('background pause does not inject simulation ticks; explicit reset clears backlog', () => {
  const clock = createFixedStepClock();
  clock.addFrame(1000 / 120);
  const before = clock.snapshot().accumulatorMS;
  // Background: the production ticker is stopped, so no addFrame is invoked.
  assert.equal(clock.snapshot().accumulatorMS, before);
  assert.equal(clock.hasPendingStep(0), false);
  clock.addFrame(1000 / 120);
  assert.equal(clock.hasPendingStep(0), true);
  clock.completedStep();
  clock.finishFrame(1);
  assert.equal(clock.snapshot().accumulatorMS, 0);
  clock.addFrame(5);
  clock.reset();
  assert.equal(clock.snapshot().accumulatorMS, 0);
});
