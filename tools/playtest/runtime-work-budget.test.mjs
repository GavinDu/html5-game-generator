import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createSimulationWorkBudget, createSemanticInputQueue, SIMULATION_WORK_LIMITS, MAX_QUEUED_SEMANTIC_INPUTS, shouldRunSemanticTicker } from '../../internal/api/web/runtime-work-budget.js';

const runtime = await readFile(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');

test('bounded rule and action checks fail deterministically on first excess operation', () => {
  const work = createSimulationWorkBudget({ maxChecks: 2, maxActions: 1 });
  work.consumeCheck(); work.consumeCheck(); work.consumeAction();
  assert.deepEqual(work.snapshot(), { checks: 2, actions: 1 });
  assert.throws(() => work.consumeCheck(), /rule evaluation work budget exceeded/);
  assert.throws(() => work.consumeAction(), /rule action work budget exceeded/);
  assert.deepEqual(work.snapshot(), { checks: 2, actions: 1 }, 'failed work must not mutate the counter');
});

test('reset creates a new fixed-step work allowance without history-dependent behavior', () => {
  const work = createSimulationWorkBudget({ maxChecks: 1, maxActions: 1 });
  work.consumeCheck(); work.consumeAction(); work.reset();
  assert.deepEqual(work.snapshot(), { checks: 0, actions: 0 });
  work.consumeCheck(); work.consumeAction();
  assert.deepEqual(work.snapshot(), { checks: 1, actions: 1 });
});

test('the production limits are finite and invalid limits cannot disable protection', () => {
  assert.deepEqual(SIMULATION_WORK_LIMITS, { maxChecks: 32768, maxActions: 8192 });
  for (const invalid of [0, -1, Infinity, NaN, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => createSimulationWorkBudget({ maxChecks: invalid, maxActions: 1 }), RangeError);
    assert.throws(() => createSimulationWorkBudget({ maxChecks: 1, maxActions: invalid }), RangeError);
  }
});

test('interpreter counts before rule matching and action execution, with terminal error handling', () => {
  assert.match(runtime, /workBudget\.consumeCheck\(\);\s*if \(!ruleMatches/);
  assert.match(runtime, /for \(const action of actions\) \{\s*workBudget\.consumeAction\(\);/);
  assert.match(runtime, /function tickFrame\(dtMs\) \{\s*if \(state\.ended \|\| state\.failed\) return;\s*workBudget\.reset\(\)/);
  assert.match(runtime, /state\.failed = true;\s*\$\('status'\)\.textContent = 'Game cannot continue safely/);
  assert.match(runtime, /if \(state\.depth > 8\) throw new Error\('rule event cascade depth budget exceeded'\)/);
  assert.match(runtime, /function emitScoreThresholds\(\) \{\s*for \(const rule of ir\.rules\) \{\s*workBudget\.consumeCheck\(\)/);
  assert.match(runtime, /if \(rule\.when\?\.event !== 'timer_elapsed'\) continue;/);
  assert.match(runtime, /function endGame\(won\) \{\s*if \(state\.ended \|\| state\.failed\) return;/);
});


test('semantic inputs have bounded FIFO order, no silent drop, and fresh-tick replay', () => {
  assert.equal(MAX_QUEUED_SEMANTIC_INPUTS, 128);
  const q = createSemanticInputQueue(2);
  q.enqueue({ action: 'tap-left' });
  q.enqueue({ action: 'tap-right' });
  assert.equal(q.size, 2);
  assert.throws(() => q.enqueue({ action: 'extra' }), /semantic input queue budget exceeded/);
  assert.equal(q.size, 2);
  const delivered = [];
  assert.equal(q.drain(input => {
    delivered.push(input.action);
    if (input.action === 'tap-left') q.enqueue({ action: 'next-tick' });
  }), 2);
  assert.deepEqual(delivered, ['tap-left', 'tap-right']);
  assert.equal(q.size, 1, 'reentrant input must wait for the next virtual tick');
  assert.equal(q.drain(input => delivered.push(input.action)), 1);
  assert.deepEqual(delivered, ['tap-left', 'tap-right', 'next-tick']);
  q.enqueue({ action: 'held' });
  q.clear();
  assert.equal(q.size, 0, 'background and canceled input must not survive resume');
});

test('queue capacity cannot be bypassed with malformed definitions', () => {
  for (const invalid of [-1, 0, NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => createSemanticInputQueue(invalid), RangeError);
  }
});

test('all live and replay input rules run inside one fixed-step work budget', () => {
  assert.match(runtime, /function tickFrame\(dtMs\) \{\s*if \(state\.ended \|\| state\.failed\) return;\s*workBudget\.reset\(\);\s*semanticInputQueue\.drain/);
  assert.match(runtime, /semanticInputQueue\.drain\(\(\{ source, target, action \}\) => \{\s*emit\('input', source, target, undefined, action\);/);
  assert.match(runtime, /function applyReplayInputs\(step\) \{[^]*?queueGameInput\(playerEnt, undefined, tapActionName\(\)\);/);
  assert.match(runtime, /function queueGameInput\(source, target, action\) \{[^]*?semanticInputQueue\.enqueue/);
  assert.match(runtime, /const releaseHeldInputs = \(\) => \{[^]*?semanticInputQueue\.clear\(\)/);
  assert.match(runtime, /if \(state\.ended \|\| state\.failed\) return;\s*try \{\s*semanticInputQueue\.enqueue/);
});


test('ticker resumes only for a live visible, context-valid interactive session', () => {
  const runnable = { shellStarted: true, hidden: false, orientationBlocked: false,
    contextLost: false, ended: false, failed: false, renderFailed: false,
    deterministicCapture: false };
  assert.equal(shouldRunSemanticTicker(runnable), true);
  for (const stopCondition of ['hidden', 'orientationBlocked', 'contextLost',
    'ended', 'failed', 'renderFailed', 'deterministicCapture']) {
    assert.equal(shouldRunSemanticTicker({ ...runnable, [stopCondition]: true }), false,
      stopCondition + ' must fail closed');
  }
  assert.equal(shouldRunSemanticTicker({ ...runnable, shellStarted: false }), false);
  assert.equal(shouldRunSemanticTicker(), false);
  assert.equal(shouldRunSemanticTicker({ ...runnable, contextLost: true, hidden: true }), false);
  assert.equal(shouldRunSemanticTicker({ ...runnable, contextLost: false }), true,
    'restored context is eligible only with every other guard still true');
});

test('visibility, context loss, rotation and capture cancellation share a guard and clear inputs', () => {
  assert.match(runtime, /document\.addEventListener\('visibilitychange', \(\) => \{\s*if \(document\.hidden\) \{\s*releaseHeldInputs\(\)/);
  assert.match(runtime, /webglcontextlost[^]*?contextLost = true;\s*releaseHeldInputs\(\);\s*try \{ app\.ticker\.stop\(\)/);
  assert.match(runtime, /webglcontextrestored[^]*?contextLost = false;[^]*?resumeTickerIfSafe\(\)/);
  assert.match(runtime, /lostpointercapture', \(\) => \{[^]*?if \(pointer\.down\) releaseHeldInputs\(\)/);
  assert.match(runtime, /pointercancel', \(\) => releaseHeldInputs\(\)/);
  assert.match(runtime, /if \(orientationBlocked\) releaseHeldInputs\(\);\s*resumeTickerIfSafe\(\)/);
  assert.match(runtime, /resumeTickerIfSafe = \(\) => \{\s*const canRun = shouldRunSemanticTicker/);
  assert.match(runtime, /if \(canRun\) app\.ticker\.start\(\);\s*else app\.ticker\.stop\(\)/);
});
