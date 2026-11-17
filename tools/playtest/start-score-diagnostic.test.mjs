import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { conditionsMet, ruleMatches } from '../../internal/api/web/runtime-rule-predicates.mjs';

const fixturePath = new URL('../../internal/attestation/testdata/start_score_ir_v1.json', import.meta.url);
const expectedSHA256 = '9751d51614b4eb1b9bf8f03d155b8526c4f21f116b38d9af90b7ef6346af855d';
const capability = 'go-scalar-start-score-v1';

// Renderer-independent PREINSTALLED rule predicates only. This is a source
// semantic golden, not a Pixi canvas boot, server-issued replay or trusted
// player score. The separate #469 must test real Chromium/WebKit Pixi output.
function runPreTickStartSemanticFixture(ir) {
  const state = { score: 0, health: 3, elapsed: 0, variables: {} };
  const digest = createHash('sha256');
  digest.update(capability + '\n');
  let actions = 0;
  for (const [ruleIndex, rule] of ir.rules.entries()) {
    if (!ruleMatches(rule, 'game_start', undefined, undefined, undefined, undefined) ||
        !conditionsMet(rule, state)) continue;
    for (const [actionIndex, action] of rule.actions.entries()) {
      assert.equal(action.type, 'add_score', 'fixture must remain narrow');
      assert.ok(Number.isSafeInteger(action.value) &&
        Math.abs(action.value) <= 1_000_000, 'no permissive JS coercion in subset');
      state.score += Math.trunc(action.value);
      actions += 1;
      digest.update([
        ruleIndex, actionIndex, rule.id, action.value, state.score,
      ].join(':') + '\n');
    }
  }
  return { score: state.score, actions, sha256: digest.digest('hex') };
}

test('Go vs preinstalled JS rule predicates share pinned authored-order start-score fixture', () => {
  const ir = JSON.parse(readFileSync(fixturePath, 'utf8'));
  assert.equal(ir.irVersion, '1.0');
  assert.equal(ir.scenes.length, 1);
  assert.equal(ir.inputBindings.length, 0);
  assert.deepEqual(runPreTickStartSemanticFixture(ir), {
    score: 1, actions: 2, sha256: expectedSHA256,
  });
  assert.deepEqual(runPreTickStartSemanticFixture(ir), runPreTickStartSemanticFixture(ir),
    'start-event diagnostic must be replay/restart deterministic');
});

test('preinstalled JS rule order trace detects reorder even when final score is equal', () => {
  const ir = JSON.parse(readFileSync(fixturePath, 'utf8'));
  const first = runPreTickStartSemanticFixture(ir);
  ir.rules.reverse();
  const second = runPreTickStartSemanticFixture(ir);
  assert.equal(first.score, second.score);
  assert.notEqual(first.sha256, second.sha256,
    'a commutative score must not hide different authored rule order');
});

test('fractional JavaScript coercion is outside the deliberately admitted Go profile', () => {
  const ir = JSON.parse(readFileSync(fixturePath, 'utf8'));
  ir.rules[0].actions[0].value = 2.9;
  assert.throws(() => runPreTickStartSemanticFixture(ir), /no permissive JS coercion/,
    'Go preflight rejects JS Math.trunc floating literals, not silently truncates them');
});
