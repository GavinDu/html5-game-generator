# Penguin's Heartlight Rescue

## Source interpretation

Created from the three supplied JPEG photographs of a child's penguin coloring pages. The repeated character anchors are a round penguin, oversized circular eyes, white belly, tiny golden beak, short flippers, webbed feet and a striped winter scarf. The three pages contribute three distinct looks:

1. **Rainbow penguin:** multicolor head/feathers and a rainbow-striped scarf.
2. **Holiday penguin:** rainbow coloring, little heart/feather details and a festive scarf.
3. **Classic penguin:** black feathers, white belly, blue striped scarf and golden feet.

The game preserves those recognizable details while inventing a snowy aurora setting and a gentle collection quest. The generated sprites are stylized interpretations, not claims that the child drew the added scenery.

## Story

The winter sky has lost its six colors. A little penguin discovers that floating hearts carry warm light. With every pair of hearts, one band of the aurora wakes up. Help the penguin gather twelve hearts and bring the whole sky back to life.

## Filled creation questionnaire

| Question | Answer |
|---|---|
| Working title | Penguin's Heartlight Rescue |
| Audience | Children ages 4–8; family-friendly |
| Genre | Cozy collection arcade / light exploration |
| Theme | Winter wonderland, friendship, warmth and color |
| Main character | Round penguin with large eyes, golden beak and webbed feet |
| Character variants | Rainbow scarf, festive scarf, classic blue scarf |
| Core objective | Catch 12 floating hearts and restore six aurora bands |
| Core loop | Tap to guide penguin → catch hearts → light a color band → celebrate progress |
| Controls | One-finger tap to move horizontally; large forgiving catch area |
| Session length | About 45–60 seconds |
| Difficulty | Gentle speed increase; no lives, timer pressure or failure state |
| Rewards | A color-band celebration every two hearts; scarf appearance changes at 4 and 8 hearts |
| Win condition | All 12 hearts collected and all six aurora colors restored |
| Lose condition | None; missed hearts simply drift away and new hearts continue to appear |
| Visual style | Soft winter palette, rounded shapes, high-contrast penguin and colorful scarf |
| Audio | Calm synthesized background loop and soft rising chimes on collection |
| Orientation | Portrait, designed for phone and tablet |
| Accessibility | Single input, no time limit, no penalty for misses, concise instructions, clear progress counter |
| Replay | Restart from the player result state; vary heart positions and golden-heart bonus spawns |
| Social response | Share the restored aurora result; remix by changing scarf palette, setting or target |
| Multiplayer | Not required for the first version |
| Safety | No external links, chat, purchases, ads, scary hazards or user-generated text |

## Gameplay details

- Tap left or right to guide the penguin across the bottom of the playfield.
- Pink hearts and occasional golden hearts drift down from the sky.
- Catching a normal heart awards 10 points; a golden heart awards 30.
- Every two catches restore one of six aurora bands.
- At four hearts the penguin changes to the festive scarf; at eight it changes to the classic blue scarf.
- Misses have no life or score penalty. Hearts keep spawning until the aurora is complete.
- Twelve catches trigger a short three-note celebration and a successful result.

## Quality goals

- The first collectible should be reachable within the first few seconds.
- The player should be visually distinct from collectibles and background.
- The progress counter must remain legible on narrow phones and larger iPads.
- Catch detection should be generous and consistent at 60 Hz.
- The game must never softlock because a heart was missed.
- Keep the scene portrait-first and avoid small precision targets.


## Portrait polish and stability revision — 2026-11-06

This is a portrait-only game (`game.orientation = "portrait"`). It does not require a second landscape layout. The player preserves the authored aspect, pauses in an unsupported viewport orientation, and guides the player to rotate; returning resumes the same session.

The original three penguin variants remain. The scene now uses a moonlit winter palette, snowy mountain layers and an ice shelf. Dim aurora ribbons brighten after each pair of catches, with explicit alpha restoration so the reward is visible. Short taps move along the fixed horizontal lane without a platformer controller. Collectibles are sensors and recycle after misses, preserving the no-fail promise. Each catch adds a soft chime and a small particle burst. All twelve catches, six bright bands, scarf transitions and restart are verified with real touch input in both Chromium and WebKit.
