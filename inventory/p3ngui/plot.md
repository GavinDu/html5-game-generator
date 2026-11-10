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

## Collection-score fidelity repair — 2026-11-07

The heart count is independent of bonus score. Every real caught heart increments `hearts_collected` once; regular pink hearts award 10 points, and two yellow-tinted golden hearts in the twelve-heart field award 30 points. The same 2/4/6/8/10/12 heart thresholds unlock aurora bands and source-inspired scarf variants regardless of point bonuses. The win still requires twelve catches, not a score threshold. No penalty is imposed for misses.

## v3 screenshot readability correction — 2026-11-09

Real iPhone and iPad screenshot review found multiple floating hearts crossing
the introductory title and tap instructions. Version 3 adds a high-contrast,
chamfered *visual-only* instruction panel above the moving pickups and below
the title/hint painter order. It never changes the authored 112×134 penguin
collision area, heart paths, score, timers, or six aurora milestones.
Chromium/WebKit phone/tablet screenshots and package-z-index assertions
are required before this is accepted. The prior v2 poster remains explicitly
version-bound; no media is silently rebound to v3.

## Version 4 — three source-character variants (2026-11-09)

Comparison of the three submitted penguin drawings against the successful
iPhone/iPad Chromium screenshot evidence showed that the rainbow and holiday
skins were almost indistinguishable except for scarf stripes. The first drawing
has a striped multicolor head and patchwork body, the second has a festive
purple/green/yellow/pink crown with cooler-toned flippers, and the third is
the classic dark-head penguin with a blue scarf and warm feet.

Version 4 replaces only the **rainbow and holiday** sprite PNGs with cleaned
original interpretations of those supplied color patterns. The classic skin
already preserved its source treatment and is retained. Photographed paper,
handwritten labels and watermarks are excluded. The canonical hero collider
remains 112×134, horizontal tap movement and 1.5× render scale are unchanged.
The 4- and 8-heart skin transitions, aurora thresholds and score still follow
the existing questionnaire exactly.

The per-variant palette/shape test is a *hard source-fidelity regression*:
rainbow and holiday must have colorful crowns distinct from one another,
classic must keep its dark crown, transparent paper-free borders and an opaque
readable face. CI must also capture the three **actual gameplay** milestone
screenshots on iPhone/iPad in Chromium and WebKit and retain them for human
comparison. Bump manifest 3→4; do not attach old preview posters to new IR/art.

### Texture footprint corrected after screenshot evidence

The first Chromium iPhone/iPad v4 screenshot candidates showed the rainbow and
holiday variants noticeably smaller than the unchanged classic variant even
though gameplay collision remained the same. Their first generated PNGs used a
smaller 128×148 natural texture. The player renderer's asset swap retained that
natural image-size difference. Regenerated both colorful variants at the
classic source sprite's 160×190 pixel footprint without changing IR/physics;
a new automated PNG dimension equality check prevents a silent size mismatch.
Fresh screenshot evidence from the corrected exact commit is required.

## Version 6 — reveal gameplay on first reward (2026-11-09)

The exact-head Chromium and WebKit quality screenshots for v5 showed the
full-size seven-second story card still covering the catcher's visual playfield
when an automated guided session had collected 11 of the required 12 hearts.
That scripted trace is NOT a measurement of a child's typical 55-second round.

Version 6 keeps the welcoming introduction at game start, plus the original
**seven-second timer** as a fallback when a child takes longer to catch
anything. At the first two-heart aurora milestone, the same event now makes
the opaque title panel and its large title transparent, before displaying the
Ruby aurora reward. The small persistent tap-to-glide hint remains. This
reveals real gameplay as soon as the first reward matters, without altering
spawn physics, target score, win conditions, source-inspired sprites, or
authored character collider/scale. Restart resets the introduction.

Acceptance requires unmodified **real browser screenshots** (Chromium and
WebKit, iPhone and iPad portrait) before first catch, immediately after the
two-heart reward before the fallback timer, at scarf changes, at win, and
after restart. Continue to flag rapid guided completion as an advisory and
do not silently alter the source questionnaire's 55-second target without
unscripted human play evidence. Package manifest version 6 is intentional;
old poster/preview files are not automatically approved for this new version.
