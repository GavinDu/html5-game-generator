import test from 'node:test';
import assert from 'node:assert/strict';
import { evadeDirection } from './heuristic.mjs';

test('evades a hazard on the opposite horizontal side', () => {
  assert.equal(evadeDirection({ id: 'player', x: 80 }, { id: 'hazard', x: 20 }), 1);
  assert.equal(evadeDirection({ id: 'player', x: 20 }, { id: 'hazard', x: 80 }), -1);
});

test('uses a stable entity-id tie-breaker when x coordinates align', () => {
  const player = { id: 'player-1', x: 50 };
  const hazard = { id: 'hazard-1', x: 50 };
  const first = evadeDirection(player, hazard);
  for (let i = 0; i < 100; i += 1) {
    assert.equal(evadeDirection(player, hazard), first);
  }
  assert.ok(first === -1 || first === 1);
});
