import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createSimulationWorkBudget, createSemanticInputQueue, createCommittedInputWitness, MAX_COMMITTED_INPUT_WITNESS_EVENTS, SIMULATION_WORK_LIMITS, MAX_QUEUED_SEMANTIC_INPUTS, shouldRunSemanticTicker } from '../../internal/api/web/runtime-work-budget.js';

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
  assert.match(runtime, /for \(let actionIndex = 0; actionIndex < actions\.length; actionIndex\+\+\) \{\s*const action = actions\[actionIndex\];\s*workBudget\.consumeAction\(\);/);
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
  assert.match(runtime, /semanticInputQueue\.drain\(\(\{ source, target, action \}\) => \{[\s\S]*?if \(state\.ended \|\| state\.failed\) return;\s*emit\('input', source, target, undefined, action\);\s*if \(!state\.failed\) committedInputWitness\?\.record\(play\.simStep, action\);/);
  assert.match(runtime, /function applyReplayInputs\(step\) \{[^]*?queueGameInput\(playerEnt, undefined, tapActionName\(\)\);/);
  assert.match(runtime, /function queueGameInput\(source, target, action\) \{[^]*?semanticInputQueue\.enqueue/);
  assert.match(runtime, /releaseHeldInputs = \(\) => \{[^]*?semanticInputQueue\.clear\(\)/);
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
  // Live input releases on interruption; a stored trace is never mutated by
  // display focus or physical pointer cancellation during diagnostic replay.
  assert.match(runtime, /document\.addEventListener\('visibilitychange', \(\) => \{[^]*?if \(document\.hidden\) \{[^]*?if \(!replayTrace\) releaseHeldInputs\(\)/);
  assert.match(runtime, /webglcontextlost[^]*?contextLost = true;\s*if \(!replayTrace\) releaseHeldInputs\(\);\s*try \{ app\.ticker\.stop\(\)/);
  assert.match(runtime, /webglcontextrestored[^]*?contextLost = false;[^]*?resumeTickerIfSafe\(\)/);
  assert.match(runtime, /lostpointercapture', \(\) => \{[^]*?if \(!replayTrace && pointer\.down\) releaseHeldInputs\(\)/);
  assert.match(runtime, /pointercancel', \(\) => \{\s*if \(!replayTrace\) releaseHeldInputs\(\)/);
  assert.match(runtime, /if \(orientationBlocked && !replayTrace\) releaseHeldInputs\(\);\s*resumeTickerIfSafe\(\)/);
  assert.match(runtime, /resumeTickerIfSafe = \(\) => \{\s*const canRun = shouldRunSemanticTicker/);
  assert.match(runtime, /if \(canRun\) app\.ticker\.start\(\);\s*else app\.ticker\.stop\(\)/);
});


test('probe-only committed witness preserves actual same-step action order without source/target data', () => {
  assert.equal(MAX_COMMITTED_INPUT_WITNESS_EVENTS, 128);
  const witness = createCommittedInputWitness();
  const queue = createSemanticInputQueue(4);
  const source = { authorization: 'private-do-not-record' };
  queue.enqueue({ source, action: 'jump' });
  queue.enqueue({ source, action: 'jump' }); // OS repeat is a second executed action
  queue.enqueue({ source, action: 'move_left' });
  assert.equal(queue.drain(({ action }) => witness.record(7, action)), 3);
  assert.deepEqual(witness.snapshot(), {
    complete: true, failureReason: null, accepted: 3,
    entries: [
      { tick: 7, ordinal: 0, action: 'jump' },
      { tick: 7, ordinal: 1, action: 'jump' },
      { tick: 7, ordinal: 2, action: 'move_left' },
    ],
  });
  assert.doesNotMatch(JSON.stringify(witness.snapshot()), /authorization|private-do-not-record/);
  const exported = witness.snapshot();
  exported.entries[0].action = 'malicious';
  assert.equal(witness.snapshot().entries[0].action, 'jump',
    'external diagnostic consumers cannot rewrite internal fixed-tick witness');
});

test('160 interrupted/reentrant tick schedules never claim an unexecuted input', () => {
  for (let cycle = 0; cycle < 160; cycle++) {
    const queue = createSemanticInputQueue(3);
    const witness = createCommittedInputWitness();
    queue.enqueue({ action: 'win' });
    queue.enqueue({ action: 'ghost' });
    let ended = false;
    queue.drain(({ action }) => {
      if (ended) return;
      if (action === 'win') ended = true;
      witness.record(cycle, action);
    });
    assert.deepEqual(witness.snapshot().entries, [
      { tick: cycle, ordinal: 0, action: 'win' },
    ]);
    queue.enqueue({ action: 'cancelled' });
    queue.clear(); // OS background/pointercancel before a fixed tick
    assert.equal(queue.size, 0);
    assert.equal(witness.snapshot().accepted, 1);
  }
});

test('committed witness caps 128 rows and fails incomplete on bad ticks/actions', () => {
  for (const invalid of [0, -1, 129, 1.5, NaN, Infinity]) {
    assert.throws(() => createCommittedInputWitness(invalid), RangeError);
  }
  const full = createCommittedInputWitness();
  for (let n = 0; n < MAX_COMMITTED_INPUT_WITNESS_EVENTS; n++) {
    assert.equal(full.record(n, 'key_repeat'), true);
  }
  assert.equal(full.record(128, 'key_repeat'), false);
  assert.deepEqual(full.snapshot(), {
    complete: false, failureReason: 'witness_limit', accepted: 128, entries: [],
  });
  assert.equal(full.record(129, 'other'), false);

  const invalidCases = [
    [[-1, 'jump']], [[1.5, 'jump']], [[108001, 'jump']],
    [[2, 'jump'], [1, 'jump']], [[1, 'jump/action']], [[1, 'é']],
    [[0, '']], [[0, 'x'.repeat(129)]], [[0, null]],
  ];
  for (const events of invalidCases) {
    const witness = createCommittedInputWitness();
    for (const [tick, action] of events) witness.record(tick, action);
    assert.equal(witness.snapshot().complete, false);
    assert.deepEqual(witness.snapshot().entries, [],
      'invalid committed events cannot leave a truthful-looking partial trace');
  }
});

test('live Pixi only samples post-emit committed inputs when probe mode is explicitly enabled', () => {
  assert.match(runtime, /const committedInputWitness = probeMode \? createCommittedInputWitness\(\) : null;/);
  assert.match(runtime, /if \(committedInputWitness\) play\.committedInputWitness = \(\) => committedInputWitness\.snapshot\(\);/);
  assert.match(runtime, /emit\('input', source, target, undefined, action\);\s*if \(!state\.failed\) committedInputWitness\?\.record\(play\.simStep, action\);/);
  assert.doesNotMatch(runtime, /replayInputRecorder\.record\([^)]*committedInputWitness/);
  assert.doesNotMatch(runtime, /submitRun\([^)]*committedInputWitness/);
  assert.doesNotMatch(runtime, /sendNativeMetric\([^)]*committedInputWitness/);
  assert.doesNotMatch(runtime, /sendTelemetryEvent\([^)]*committedInputWitness/);
});
