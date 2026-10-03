# Escape from the Injection

From a kid's hand-drawn storyboard (photo, 2026-10-03). The drawing shows a
big white blob about to get a syringe in the butt, fleeing from a small
spiky-haired figure, with chase arrows — plus a handwritten three-step plot:

1. **"Inject in the butt"** — the blob is cornered in the clinic, the shot is coming.
2. **"escape further and further"** — the blob flees, the chaser falls behind, then closer...
3. **"finally, he is caught and Injected with tears"** — no one outruns the shot forever. 😭

## Elements extracted from the drawing

| Drawing element | In the game |
|---|---|
| The big white blob (worried face) | The hero you steer: a cute-cartoon white blob, redrawn dress-up-duck style (thick outlines, blush cheeks) |
| The syringe | The threat: cartoon syringes rain down the escape route — dodge them |
| The small spiky-haired figure | The chaser: a determined kid with a syringe who trails behind you and lunges when he gets close |
| The chase arrows | The gap between blob and chaser — your real health bar |
| The 3-step handwritten plot | The game structure: cornered → escape further and further → finally caught |

## Extended plot

The blob escapes the clinic, but the chaser never gives up. Every 40 seconds
the blob flees somewhere new — Clinic 🏥 → Hallway 🏃 → Playground 🛝 →
Park 🌳 → Beach 🏖️ → Mall 🛍️ → Nowhere Left 😱 — and everything gets
quicker. Courage stars ⭐ along the route buy back some lead. But the plot
is fixed, like the drawing says: eventually the chaser catches up, and it's
a shot in the butt... with tears.

## Rules

- **Tap LEFT / RIGHT side** of the screen to switch lanes (3 lanes).
- **Dodge syringes** 💉 — each hit lets the chaser gain 45px on you.
  Syringes never spawn as an unavoidable 3-lane wall: when two lanes already
  have syringes arriving together, new ones stack into those lanes.
- **Grab courage stars** ⭐ — +25 score, and push the chaser back 12px.
- **The gap is your health**: it starts at 220px, recovers 6px/s while you run
  clean, and if it shrinks to 55px the chaser lunges — game over.
- **Every 40 seconds**: next level, 1.12× faster (capped at 3.2×), new backdrop.
- **Scoring**: 10/sec of running + 25/star. Lose ends the run; the leaderboard
  ranks your best escape by **level reached**, then score.
- A skilled run lasts roughly 15–20 minutes — a short game, as requested.

## Escape strategy (Poki-style chase notes)

Borrowed from the endless-chase shelf (Subway Surfers school of design):

1. **Own the middle lane.** From the center you can reach either side in one
   tap — the cheapest insurance in the game.
2. **Stars are risk/reward.** They spawn in random lanes, often where a
   syringe is falling. Only chase one when the gap is healthy.
3. **Read two rows ahead.** At higher speed, react to the *next* syringe, not
   the one at your feet — same skill as reading ahead in a runner.
4. **Play safe when the gap is thin.** Below ~100px, skip stars and just dodge.

## Art

All sprites drawn procedurally in `draw_sprites.py` in a cute rounded-cartoon
style (thick black outlines, flat bright fills, blush cheeks), following the
look of Poki's dress-up-duck — while keeping every element from the kid's
drawing: the blob, the syringe, the spiky-haired chaser.

## v1 (2026-10-03)

New game. Tap-to-dodge 3-lane escape with the visible chaser-gap mechanic,
level-up-every-40-seconds pacing, and leaderboard integration (best run per
player, ranked by level then score).
