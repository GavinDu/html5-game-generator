# Fixed-step pointer and keyboard replay parity (v1 compatibility)

Parent: [Hephaestus Epic #202](https://github.com/GavinDu/Hephaestus/issues/202) · [Generator semantic parity #315](https://github.com/GavinDu/html5-game-generator/issues/315) · [Server attestation #360](https://github.com/GavinDu/html5-game-generator/issues/360).

## Confirmed divergence

The live WebGL canvas reports each physical pointerdown with `noteInput('pointer', coordinates)`, then the fixed-step recorder periodically adds *another* `pointer` event while the contact is held, describing drag movement. Historically playback called `queueGameInput(tapActionName())` for **every** `pointer`, inventing repeated tap/rule actions while dragging and repeated lane transitions. It also set the replay keyboard state without enqueueing the corresponding discrete keyboard action (or the lane-tap movement side effect). `pointerup` was formerly recorded only from the next ticker step after release; two rapid taps entirely between ticks could have no separating `pointerup` in their trace.

## Immediate compatibility correction

- Keep the v1 `pointer` / `pointerup` / `key` / `keyup` wire vocabulary. Interpret the **first** `pointer` after an up as a physical down and subsequent `pointer` samples while held as moves. Only a down enqueues the tap action.
- Share `beginPointer` (lane target, x-only constraints, tap destinations) and `beginKeyboardAction` (keyboard lane change, key-held state and discrete action queue) between live DOM events and replay.
- Record a release **on the physical pointerup edge**, including fast down/up inside one 60 Hz tick. Explicitly record cancellation/blur releases for held inputs where available. Do not duplicate release at the next ticker step. Same-tick event order remains stable.
- Suppress live pointer/key/focus, visibility, context-loss and orientation release mutations while a stored trace owns the replay control state; pausing presentation never silently alters replay inputs.
- Reuse the bounded 2,000-event recorder and the existing strict server input grammar; do not add a new privileged event or dynamic code capability.

## Senior game-engine-architect challenge: constraints still open

This is a **partial v1 producer/consumer compatibility repair**, not a proven deterministic or server-verified outcome. Old persisted traces can lack release events; they cannot be reconstructed truthfully. The v1 pointer type still overloads down versus movement, and multi-touch, blur-triggered pending-queue cancellation, pointer capture loss, same-tick event ordering across asynchronous callbacks, keyboard OS autorepeat and viewport transform changes need an explicit **v2 typed input protocol**. The old scoreboard claims remain unverified.

Next design: versioned `pointer_down(pointerId,x,y)`, `pointer_move(pointerId,x,y)`, `pointer_up(pointerId)`, `pointer_cancel(pointerId)`, `key_down(code,repeat)`, `key_up(code)`, and a canonical `cancel_pending_actions` event. Normalize coordinates through the immutable Game IR viewport transform. Enforce one active pointer unless negotiated; count bounded events, reject unsupported fields and unmatched physical transitions (including unbound keyboard). Drive both live and replay through a **single** tick-ordered command reducer; never mutate the simulation directly from a DOM event. Version physics/RNG deterministically and compare golden semantic state/score per tick over all shipped genres. Reject or flag legacy traces as non-attestable instead of silently inferring missing edges. Finally, implement the Go independent trusted replay interpreter in #360 before any `Verified=true` transition.

Required CI: Go tests (including race), PostgreSQL, Chromium, WebKit, cross-browser determinism, Playability Kit at the exact head. Physical iPhone/iPad instrumented gesture comparison remains separate, pending.
