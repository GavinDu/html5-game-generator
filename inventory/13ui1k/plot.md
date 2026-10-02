# Bobo's Rainbow — plot & design

Source: a kid's crayon drawing (two photos of the same sheet).

## Elements read from the drawing

| # | Element | What the kid drew |
|---|---------|-------------------|
| 1 | **Bobo** | A big brown furry bear with wide blue eyes, wearing a tiny golden crown topped with little hearts — the Bear Prince |
| 2 | **Hearts** | Pink and blue hearts floating in the sky around him |
| 3 | **Rainbow** | A big rainbow arching across the bottom of the page, its colors vivid |
| 4 | **Kitten** | A small pencil sketch of a kitten under the rainbow |
| 5 | **Flower** | A tiny flower beside the kitten |
| 6 | **Orange** | A small orange fruit floating by Bobo's side |

## Main plot — "Bobo's Rainbow"

High above the meadow lives **Bobo**, the little Bear Prince, in his crown
of hearts. Below him arches the great **rainbow bridge** — the road the tiny
**kitten** walks to come visit, past the little **flower**.

One morning the rainbow wakes up faded. Its six color bands are going grey,
one by one, and if the last band fades the kitten won't be able to cross.

But the **hearts** drifting down from the sky hold the rainbow's magic.
Bobo hurries along the bottom of the sky, catching falling hearts. **Every
2 hearts repaints one band of the rainbow** — red, orange, yellow, green,
blue, purple. Catch **12 hearts**, and the rainbow shines whole again: the
kitten crosses, the flower blooms, everybody celebrates.

Kid-safe by design: there is no way to lose — hearts keep falling until the
rainbow is complete.

## Mechanics (GameShim v1)

- **Controls:** tap (click / touch) — Bobo slides left/right to the tap point
- **Loop:** hearts (pink + blue, from the drawing) fall from the top of the
  sky; catching one plays a rising tone and adds to the score
- **Progress:** every 2 hearts restores the next rainbow band with a
  "Red restored!"-style banner
- **Win:** 12 hearts → rainbow complete → `Shim.end({ won: true })`
- **Music:** calm loop (synth placeholder for the licensed-library matcher)

## Alternate plots (not built)

1. **Crown of Hearts** — the hearts have fallen out of Bobo's crown; catch
   pink and blue hearts in matching pairs to rebuild it. (Memory/matching
   variant.)
2. **Kitten's Crossing** — the rainbow is whole but wobbly; keep the kitten
   balanced on the arch by tapping left/right as it walks across. (Balance
   variant.)
3. **The Lost Orange** — Bobo's little orange fruit has rolled under the
   rainbow; find it behind rain-clouds by tapping clouds away. (Seek/hide
   variant.)

## v2 revision (2026-10-02) — Poki-inspired engagement pass
Kept the kid's art and the tap-to-move core; borrowed Poki's casual-loop
formula (Subway Surfers / Stickman Hook school: score chase, streaks,
power-ups, escalating speed, instant retry):
- **Combo multiplier**: consecutive catches without a miss build COMBO x2–x5;
  each heart scores 10 × combo. A miss resets the combo.
- **Stakes**: 3 missed hearts end the round (❤❤❤ display) — the gentle
  "one more try" loop.
- **Golden heart power-up** (procedural sprite, 12% spawn): +50, never
  penalized if missed.
- **Speed ramp**: hearts fall faster and spawn quicker as the catch count rises.
- **Star rating**: ★ thresholds at 150 / 300 / 450, shown on the canvas and in
  end stats.
The 12-heart rainbow restoration and kitten-crossing win stay untouched.

## v3 revision (2026-10-02) — explicit level-up pacing
Bobo now has 6 explicit levels (one per 2 hearts caught, announced with
"LEVEL X!" + rising tone). Fall speed scales multiplicatively per level
(175 → 352 px/s, 1.15x per level) and spawn interval shrinks (0.8 → 0.4s),
so the game gets noticeably quicker and quicker as you level up.
