# Authored game orientation

`Game IR.game.orientation` is the canonical per-game flag. It is required and validated against `portrait`, `landscape`, or `any`. A game is not required to support both orientations.

| Value | Gameplay layout | Opposite viewport |
|---|---|---|
| `portrait` | Authored 390×844 portrait stage | Preserve aspect, pause, show rotate guidance, then resume the same session. |
| `landscape` | Authored 844×390 landscape stage | Preserve aspect, pause, show rotate guidance, then resume the same session. |
| `any` | Native aspect selected from the authored entry-scene dimensions | Both viewport orientations are supported; the stage keeps its authored aspect. |

The stage dimensions describe a logical viewport. A larger scene can still scroll through the camera. Rendering and input mapping use the same dimensions. The feed and package expose additive `orientation` fields derived from Game IR, so clients can label the game before launch without creating a second mutable orientation setting. Legacy records without IR retain the previous portrait default. Unknown orientation values fail validation/launch.

The GameSpec questionnaire offers all three values. Published games and generic remixes retain their canonical Game IR orientation; choosing `any` is an explicit author declaration, not a default promise that every game has two layouts.

The web player evaluates its actual available viewport, including the WKWebView viewport supplied by a native host. This is a player layout policy, not an unconditional operating-system rotation lock. Native feed badges show the game declaration. Gameplay is never stretched or reinterpreted merely because the surrounding feed rotates.

## Evaluation

- Phone/tablet gameplay checks score only the orientations the game declares.
- Opposite orientations are checked separately as `rotation-fallback`: real first frame, preserved aspect, pause and readable guidance. They are not assigned gameplay design scores.
- Rotation policy browser tests cover portrait, landscape and any fixtures, paused simulation time and session-preserving resume in Chromium and WebKit.
- The quality bot starts in a supported viewport and treats a guided pause during unsupported rotation as expected, then verifies resumed rendering, input spam and network interruption.
- Replay probes and default preview capture use supported dimensions. An explicit preview viewport that conflicts with a fixed orientation is rejected.

Penguin's Heartlight Rescue is explicitly portrait-only. Its phone/tablet portrait layout is the optimized experience; landscape only needs the safe paused fallback.
