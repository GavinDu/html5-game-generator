// "Mimic the user" brain, level 2: an LLM player. NOT exercised in the
// prototype (no model access from the build sandbox), so treat it as a
// contract, not a tested component.
//
// Why a second brain: the heuristic brain knows "collect pickups, avoid
// hazards". It cannot read a custom objective, a text instruction, a
// puzzle, or notice that something looks wrong. An LLM can, because it
// knows both the generated game (IR, plot, objective) and this kit.
//
// Cost control: call the model every ~500 ms of game time at most, with a
// compact snapshot, and only attach a screenshot every few turns or when
// the heuristic brain reports "stuck". Everything else stays on the
// heuristic brain.

export const SYSTEM_PROMPT = `You are a QA player testing a mobile browser game before it ships.
You are given the game's design (title, objective, controls) and, each turn,
a JSON snapshot of the world (and sometimes a screenshot).
Play like a real, slightly clumsy new player: read the objective, try the
documented controls, pursue the goal, and avoid hazards.
Reply with ONE JSON object:
{"action": {"type":"drag","x":<number>,"y":<number>} | {"type":"tap","x":<number>,"y":<number>} | {"type":"key","key":"ArrowLeft"} | {"type":"wait"},
 "intent": "<one short sentence>",
 "issue": null | {"severity":"blocker|major|minor","what":"<observed problem>","evidence":"<what you saw>"}}
Report an issue ONLY for something a player would experience as broken:
controls that do nothing, objects you cannot reach or collect, a goal you
cannot complete, text that is cut off or unreadable, the game freezing,
the end screen missing or wrong. Do not report taste.`;

export function buildTurn({ design, snapshot, lastIntent, screenshotB64 }) {
  return {
    design,                       // { title, objective, controls, plot }
    snapshot,                     // from window.__dqTest.snapshot()
    lastIntent,
    screenshot: screenshotB64 || null,
  };
}

// Plug your model client here. Must return the parsed JSON object above.
export function llmBrain(callModel) {
  let lastIntent = '';
  return {
    name: 'llm',
    async decideAsync(ctx) {
      const reply = await callModel(SYSTEM_PROMPT, buildTurn({ ...ctx, lastIntent }));
      lastIntent = reply?.intent || '';
      return reply;
    },
  };
}
