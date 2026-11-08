import test from 'node:test';
import assert from 'node:assert/strict';
import { outcomeCoverage, observedVictory } from './outcome-coverage.mjs';

const caught = [{ actions: [{ type: 'end_game', value: false }] }];
const exp = { controls: { scheme: 'lane_tap' }, chaserGap: { caughtAt: 55 },
  result: { lose: { title: 'Caught!' } } };

test('authored loss-only chase cannot be incorrectly reported as unwinnable', () => {
  const actual = outcomeCoverage(caught, exp, { endSeen: false, winSeen: false });
  assert.deepEqual(actual.map(x => [x.checkId, x.status]), [
    ['simulation.win_reachable', 'skip'],
    ['simulation.caught_reachable', 'skip'],
  ]);
  assert.match(actual[0].evidence, /not applicable/);
  assert.equal(actual[1].actual.observedNaturally, false);
});

test('only an observed caught terminal gets a pass, never a fixture substitute', () => {
  const actual = outcomeCoverage(caught, exp, { endSeen: true, winSeen: false });
  assert.equal(actual[0].status, 'skip');
  assert.equal(actual[1].status, 'pass');
  assert.equal(actual[1].actual.observedNaturally, true);
});

test('conventional win games retain existing pass/skip rules', () => {
  const win = [{ actions: [{ type: 'end_game', value: true }] }];
  const ordinary = { controls: { scheme: 'tap' }, result: { win: { title: 'You win' } } };
  assert.deepEqual(outcomeCoverage(win, ordinary, { winSeen: true, endSeen: true })
    .map(x => [x.checkId, x.status]), [['simulation.win_reachable', 'pass']]);
  assert.deepEqual(outcomeCoverage(win, ordinary, { winSeen: false, endSeen: false })
    .map(x => [x.checkId, x.status]), [['simulation.win_reachable', 'skip']]);
});

test('do not falsely classify generic loss rules as lane-chase stories', () => {
  const x = outcomeCoverage(caught, { controls: { scheme: 'drag' } },
    { endSeen: true, winSeen: false });
  assert.equal(x.length, 1);
  assert.equal(x[0].status, 'skip');
});

test('canonical runtime verdict wins over ambiguous/localized result titles', () => {
  assert.equal(observedVictory({ won: false, title: 'Hooray! Caught at last' }), false);
  assert.equal(observedVictory({ won: true, title: 'The end' }), true);
  assert.equal(observedVictory({ won: false, title: 'Fin' }), false);
  assert.equal(observedVictory({ won: null, title: 'Game over' }), false);
  assert.equal(observedVictory({ won: null, title: 'The rainbow is whole' }), true);
});
