# Unified UI Design System

This is the shared design contract for creator flows, discovery, game details, social actions, and the game player. The player shell in `internal/api/web/player.html` defines the initial web tokens; the iOS app maps the same roles to SwiftUI semantic colors and Dynamic Type rather than copying fixed pixel values.

## Product principles

1. **Play is the primary action.** Every game card makes Play easy to find; secondary social actions must not compete visually with it.
2. **Creator intent stays visible.** Show title, creator attribution, and remix lineage where relevant. Do not let generic decoration replace game-specific artwork once a poster/thumbnail exists.
3. **State is never silent.** Loading, empty, offline, unsupported, denied, and retryable-error states have explicit copy and a clear next action.
4. **Trust and safety are discoverable.** Report, block, not-interested, and visibility states are reachable without dominating the play surface.
5. **Progressive enhancement.** Fullscreen, haptics, animation, and advanced input are optional; their absence must not prevent play.

## Token roles

| Role | Web player baseline | Usage |
|---|---|---|
| Background | `#0b0f22` | App/player canvas background |
| Surface | `#151d37` | Navigation and primary cards |
| Raised surface | `#1b2340` | Menus, sheets, and controls |
| Text | `#ffffff` | Primary text on dark surfaces |
| Muted text | `#cfe0ff` | Supporting text on dark surfaces |
| Accent | `#ffd94d` | Focus ring and high-priority action accent |
| Border | `#465174` | Low-emphasis separation |
| Radius | `14px` | Input and control radius; cards may use a larger outer radius |
| Spacing | `4 / 8 / 12 / 16 / 24px` | Reusable spacing scale |

These are semantic roles, not permission to hard-code the same hex values into every screen. SwiftUI should prefer system background/grouped-background and foreground styles where they provide better contrast, with accent colors used consistently for primary actions and focus.

## Shared components

- **Game card:** game poster/preview, title, creator, optional caption/topics, one prominent Play action, then save/like/comment/share and overflow actions. Use a bookmark icon for saving and a heart for liking.
- **Creator identity:** one reusable avatar/name affordance; tapping opens the creator profile. Do not imply that a development placeholder ID is the authenticated account.
- **Primary action:** one high-contrast filled action per surface; use it for Play, Create, Publish, Retry, or Save changes.
- **Secondary action:** bordered or quiet button for alternate actions; icon-only controls need accessibility labels and a minimum 44×44 pt touch target on iOS.
- **Sheet/dialog:** clear title, one primary action, a visible cancel/close affordance, and no destructive default focus.
- **Loading:** progress indicator plus contextual text. After a reasonable delay, explain what is happening instead of leaving a spinner without context.
- **Empty state:** concise explanation and a useful next action (for example, clear search or create the first game).
- **Error state:** user-facing summary, retry when safe, and a non-destructive way to exit. Keep detailed diagnostics in logs/telemetry, not in the primary message.
- **Safety menu:** report, block, and not-interested actions use clear wording and confirmation where the action is consequential.

## Player shell contract

- Use `viewport-fit=cover` and safe-area insets. Never place exit, pause, or critical controls under the notch/home indicator.
- Preserve the game logical aspect ratio. Letterbox intentionally rather than stretching the game canvas.
- Use `touch-action: none` only on the active gameplay surface; normal page content should remain scrollable.
- Support portrait, landscape, resize, and orientation changes without losing the current game or stranding input.
- Fullscreen failure must degrade gracefully. The user can always exit the player.
- An offline indicator may cover a loading screen, but must not cover a game that has already loaded and remains playable.
- Loading, ready, paused, completed, unsupported-runtime, and unrecoverable-error states must be distinguishable.

## Accessibility

- Meet WCAG AA contrast for text and interactive controls.
- Support Dynamic Type, browser zoom, keyboard navigation, visible focus, VoiceOver labels, and reduced-motion preferences.
- Do not communicate state with color alone; pair status colors with text or an icon.
- Prefer simple transitions and avoid essential information that appears only on hover.
- Keep touch targets at least 44×44 pt on iOS and provide comfortable spacing between destructive actions.
- Test dark/light appearance where supported and ensure long titles, translated text, and larger fonts do not clip controls.

## Validation checklist

- [ ] Visual snapshots cover discovery cards, player loading/ready/offline/error states, and account/session state.
- [ ] Mobile web is checked at narrow portrait, short landscape, and desktop widths.
- [ ] iOS is checked on iPhone and iPad with Dynamic Type and VoiceOver enabled.
- [ ] Every icon-only action has an accessibility label and every interactive control has a visible focus state.
- [ ] A loaded game remains visible after network loss; retry resets progress and slow-load hints.
- [ ] Play and exit remain discoverable in every supported player state.
- [ ] Screenshot or browser-test failures block merge when they indicate clipped or inaccessible controls.

## Current implementation map

- Web player safe-area, responsive canvas, focus, reduced-motion, and touch styles: `internal/api/web/player.html`.
- iOS discovery card, save/like/share controls, and account session state: `GavinDu/Hephaestus` `Hephaestus/Views/AppTabs.swift`.
- iOS player loading, retry, and offline state policy: `GavinDu/Hephaestus` `Hephaestus/Player/PixiPlayer.swift`.
- This document describes the cross-platform contract; screenshots and visual regression baselines should be added as the visual test harness is expanded.
