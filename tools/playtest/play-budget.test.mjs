import test from 'node:test';
import assert from 'node:assert/strict';
import { resolvePlayBudgetSeconds } from './play-budget.mjs';

test('extended win budget is not truncated by the routine play budget', () => {
  assert.equal(resolvePlayBudgetSeconds(14, 60), 60);
});

test('a longer routine play budget remains respected', () => {
  assert.equal(resolvePlayBudgetSeconds(75, 60), 75);
});

test('equal budgets remain unchanged', () => {
  assert.equal(resolvePlayBudgetSeconds(40, 40), 40);
});
