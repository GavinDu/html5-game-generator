# Playability Kit

Plays a generated game in a real browser like a user would and returns a
verdict (**PLAYABLE / NEEDS_REVIEW / BROKEN**) with issues the repair loop
can act on. It complements the conformance worker in `tools/playtest`
(determinism, replay, shell gates): the kit is the *user-mimic gate* —
resilience, rule coverage, nonblank rendering, and issue classification.

    npm install
    PORT=18099 ./server &                       # the game backend
    node playability.mjs --base http://localhost:18099 --games sample-rainbow-bear --budget 30
    node playability.mjs ... --mutant missing-asset|no-win|runtime-throw|dead-pointer   # self-test

Checks: load.clean, load.assets, render.nonblank, control.pointer,
resil.stable, play.progress, play.win_possible, end.flow, end.restart,
run.no_errors (+ rule coverage and frame times as info).

Platform notes (adapted 2026-10-23 from the advisor prototype):

- The `window.__dqTest` snapshot hook is **native** in `pixi-runtime.js`
  under `?probe=1` (read-only; exposes positions/kinds, accepts no input).
  Route patching remains only for the `runtime-throw` mutant.
- The end screen is the IR 1.2 shell result card (`#shell-result`,
  `#shell-again-btn`); restart is in-place.
- Games start behind the shell start card — the kit taps Start like a
  user. Control checks try a drag first, then taps (tap-zone games), and
  the play loop re-presses periodically so tap-to-move games get taps.
- Chromium is resolved from `PW_CHROMIUM`, the Playwright browser cache,
  or `/opt/pw-browsers/chromium`.

`brains/heuristic.mjs` — free, fast bot (leads falling pickups, gives up
on blocked targets). `brains/llm.mjs` — contract for the LLM player
(untested: no model calls from the kit yet).
