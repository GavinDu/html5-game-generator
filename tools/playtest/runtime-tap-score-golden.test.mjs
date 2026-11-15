import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  createSemanticState,
  reduceSemanticStateTick,
  canonicalSemanticState,
} from '../../internal/api/web/runtime-semantic-state-reducer.mjs';

// Intentionally tests the renderer-free *resolved command* contract, not
// production Pixi DOM-to-rule mapping or a server-trusted score verdict.
// Go consumes the same checked-in source as canonical pointer down/up edges.
const corpus = JSON.parse(readFileSync(
  new URL('./fixtures/runtime-tap-score-golden-v1.json', import.meta.url),
  'utf8',
));
test('shared golden corpus is versioned, sufficiently diverse and immutable', () => {
  assert.equal(corpus.schema, 'runtime-tap-score-golden-v1');
  assert.ok(Array.isArray(corpus.cases));
  assert.ok(corpus.cases.length >= 7);
  assert.equal(new Set(corpus.cases.map(c => c.name)).size, corpus.cases.length);
  assert.ok(corpus.cases.every(c => typeof c.name === 'string' && c.name.length > 0));
});

for (const fixture of corpus.cases) {
  test(`pure JS semantic tick reducer matches shared Go tap-score golden: ${fixture.name}`, () => {
    const { seed, scoreDelta, durationTicks, endGame, tapTicks, expected } = fixture;
    assert.ok(Number.isInteger(seed) && seed >= 0 && seed <= 0xffffffff);
    assert.ok(Number.isInteger(scoreDelta) && Math.abs(scoreDelta) <= 1000);
    assert.ok(Number.isInteger(durationTicks) && durationTicks > 0 && durationTicks <= 108000);
    assert.ok(Array.isArray(tapTicks) && tapTicks.every((n, i) =>
      Number.isInteger(n) && n >= 0 && n <= durationTicks &&
      (i === 0 || n >= tapTicks[i - 1])));
    assert.ok(endGame === null || typeof endGame === 'boolean');

    function execute() {
      let state = createSemanticState({ seed, score: 0, health: 100 });
      let downEvents = 0;
      let actionsApplied = 0;
      // Go's v2 trace has zero-based ordinals per sim-tick. The browser
      // reducer sees already-resolved rule actions *in the same order*.
      // A pointer "up" is a balanced protocol edge, not a second rule.
      for (let tick = 0; tick <= durationTicks; tick++) {
        if (tick === 0 || state.terminal !== null) continue;
        const edges = tapTicks.filter(n => n === tick);
        const commands = [];
        for (const _edge of edges) {
          commands.push({ ordinal: commands.length,
            action: { type: 'add_score', value: scoreDelta } });
          if (endGame !== null) {
            commands.push({ ordinal: commands.length,
              action: { type: 'end_game', value: endGame } });
          }
        }
        const result = reduceSemanticStateTick(state, tick, commands);
        state = result.state;
        // Once a terminal command is reached, remaining same-tick actions
        // are deliberately ignored rather than double-scored.
        const effects = result.effects;
        downEvents += effects.filter(e => e.type === 'score_changed').length;
        actionsApplied += effects.filter(e => e.type === 'score_changed' || e.type === 'terminal').length;
      }
      return {
        score: state.score,
        terminal: state.terminal ?? 'running',
        downEvents,
        actionsApplied,
        canonical: canonicalSemanticState(state),
      };
    }
    const a = execute();
    const b = execute();
    assert.deepEqual(a, b, 'same tick/seed/order must be deterministic');
    assert.deepEqual({
      score: a.score, terminal: a.terminal, downEvents: a.downEvents,
      actionsApplied: a.actionsApplied,
    }, expected, 'shared reference outcome must be exact');
  });
}
