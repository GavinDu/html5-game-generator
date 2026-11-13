import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { conditionsMet, lookupSemanticValue, ruleMatches } from '../../internal/api/web/runtime-rule-predicates.mjs';

const state = { score: 5, health: 3, elapsed: 10, variables: { lives: 2, enabled: true, score: 99 } };
const predicate = (variable, operator, value) => ({ conditions: [{ variable, operator, value }] });

test('pure IR condition lookup preserves v1.x reserved variables, counters and time aliases', () => {
  assert.equal(lookupSemanticValue('score', state), 5, 'reserved score must win over same-named variable');
  assert.equal(lookupSemanticValue('health', state), 3);
  assert.equal(lookupSemanticValue('elapsed', state), 10);
  assert.equal(lookupSemanticValue('time', state), 10);
  assert.equal(lookupSemanticValue('lives', state), 2);
  assert.equal(lookupSemanticValue('variable:lives', state), 2);
  assert.equal(lookupSemanticValue('unknown', state), 0);
});

test('numeric comparisons retain original v1 coercion and boolean strict-equality cases', () => {
  for (const [operator, value, expected] of [
    ['eq', '5', true], ['eq', 5, true], ['eq', 6, false],
    ['neq', '5', false], ['neq', 4, true],
    ['gt', 4, true], ['gt', 5, false],
    ['gte', 5, true], ['gte', 6, false],
    ['lt', 6, true], ['lt', 4, false],
    ['lte', 5, true], ['lte', 4, false],
    ['unsupported', 5, false],
  ]) {
    assert.equal(conditionsMet(predicate('score', operator, value), state), expected,
      operator + ': ' + value);
  }
  assert.equal(conditionsMet(predicate('enabled', 'eq', true), state), true);
  assert.equal(conditionsMet(predicate('enabled', 'eq', 1), state), true,
    'v1 numeric coercion of true must remain compatible');
  assert.equal(conditionsMet(predicate('score', 'eq', true), state), false,
    'boolean RHS uses strict equality, unlike numeric RHS');
  assert.equal(conditionsMet(predicate('enabled', 'neq', false), state), true);
  assert.equal(conditionsMet(predicate('enabled', 'gt', true), state), false);
});

test('conditions are conjunctive, default empty succeeds and invalid numeric values fail closed', () => {
  assert.equal(conditionsMet({}, state), true);
  assert.equal(conditionsMet({ conditions: [] }, state), true);
  assert.equal(conditionsMet({ conditions: [
    { variable: 'score', operator: 'gte', value: 5 },
    { variable: 'lives', operator: 'lt', value: 3 },
  ] }, state), true);
  assert.equal(conditionsMet({ conditions: [
    { variable: 'score', operator: 'gte', value: 5 },
    { variable: 'lives', operator: 'gt', value: 3 },
  ] }, state), false);
  assert.equal(conditionsMet(predicate('score', 'gt', 'not-a-number'), state), false);
});

test('event match preserves source/target identity, event and same-tick action semantics', () => {
  const source = { defId: 'hero', def: { kind: 'player' }, runtimeId: 'hero#1' };
  const target = { defId: 'star', def: { kind: 'pickup' }, runtimeId: 'star#2' };
  const rule = { when: {
    event: 'collected', source: 'player', target: 'star',
    value: 3, inputAction: 'tap',
  } };
  assert.equal(ruleMatches(rule, 'collected', source, target, 3, 'tap'), true);
  assert.equal(ruleMatches(rule, 'collision', source, target, 3, 'tap'), false);
  assert.equal(ruleMatches(rule, 'collected', source, target, 4, 'tap'), false);
  assert.equal(ruleMatches(rule, 'collected', source, target, 3, 'drag'), false);
  assert.equal(ruleMatches(rule, 'collected', source, { ...target, defId: 'other' }, 3, 'tap'), false);
  assert.equal(ruleMatches({ when: { event: 'collision', source: 'hero#1', target: 'pickup' } },
    'collision', source, target), true);
  // v1.x treats absent source/target/value/inputAction as unconstrained.
  assert.equal(ruleMatches(rule, 'collected', undefined, undefined), true);
  assert.equal(ruleMatches({}, 'collected', source, target), false);
});

test('shipping Pixi interpreter uses imported semantics rather than a separate local copy', () => {
  const runtime = readFileSync(new URL('../../internal/api/web/pixi-runtime.js', import.meta.url), 'utf8');
  assert.match(runtime, /import \{ conditionsMet, ruleMatches \} from '\.\/runtime-rule-predicates\.mjs'/);
  assert.match(runtime, /ruleMatches\(rule, event, source, target, value, inputAction\)/);
  assert.match(runtime, /conditionsMet\(rule, state\)/);
  assert.doesNotMatch(runtime, /function conditionsMet\(rule\)/);
  assert.doesNotMatch(runtime, /function ruleMatches\(rule, event/);
  const module = readFileSync(new URL('../../internal/api/web/runtime-rule-predicates.mjs', import.meta.url), 'utf8');
  assert.doesNotMatch(module, /\b(?:window|document|PIXI|RAPIER|performance|fetch|Date)\b/);
});
