import SwiftUI

/// Optional on-screen controls for games that need directional or action input.
/// Events use the same semantic contract as keyboard and controller adapters.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerTouchControls: View {
    public let onInput: (PlayerInputEvent) -> Void
    public let showsDirectionalPad: Bool
    public let showsActionButtons: Bool

    public init(
        showsDirectionalPad: Bool = true,
        showsActionButtons: Bool = true,
        onInput: @escaping (PlayerInputEvent) -> Void
    ) {
        self.showsDirectionalPad = showsDirectionalPad
        self.showsActionButtons = showsActionButtons
        self.onInput = onInput
    }

    public var body: some View {
        HStack(alignment: .bottom, spacing: GamePlatformTheme.Spacing.xLarge) {
            if showsDirectionalPad {
                VStack(spacing: GamePlatformTheme.Spacing.xSmall) {
                    control("Up", symbol: "arrow.up", action: .moveUp)
                    HStack(spacing: GamePlatformTheme.Spacing.xSmall) {
                        control("Left", symbol: "arrow.left", action: .moveLeft)
                        control("Down", symbol: "arrow.down", action: .moveDown)
                        control("Right", symbol: "arrow.right", action: .moveRight)
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityLabel("Directional controls")
            }

            if showsActionButtons {
                HStack(spacing: GamePlatformTheme.Spacing.small) {
                    control("Secondary", symbol: "circle", action: .secondary)
                    control("Primary", symbol: "circle.fill", action: .primary)
                }
                .accessibilityElement(children: .contain)
                .accessibilityLabel("Action controls")
            }
        }
        .padding(GamePlatformTheme.Spacing.medium)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.large, style: .continuous))
    }

    private func control(_ label: String, symbol: String, action: PlayerInputAction) -> some View {
        Button(action: {}) {
            Image(systemName: symbol)
                .font(.headline)
                .frame(
                    minWidth: GamePlatformTheme.Control.minimumHitTarget,
                    minHeight: GamePlatformTheme.Control.minimumHitTarget
                )
                .contentShape(Rectangle())
        }
        .buttonStyle(PlayerHoldButtonStyle { pressed in
            onInput(PlayerInputEvent(
                action: action,
                phase: pressed ? .began : .ended,
                source: .touch
            ))
        })
        .accessibilityLabel(label)
        .accessibilityIdentifier("player.touch.\(action.rawValue)")
        .accessibilityHint("Press and hold to send this game control")
    }
}

/// Button style that reports the full press/release lifetime, which is needed
/// for movement controls. Unlike a tap-only action, release always emits ended.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerHoldButtonStyle: ButtonStyle {
    private let onPressChanged: (Bool) -> Void

    public init(onPressChanged: @escaping (Bool) -> Void) {
        self.onPressChanged = onPressChanged
    }

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(
                configuration.isPressed
                    ? GamePlatformTheme.Feedback.accent.opacity(0.22)
                    : GamePlatformTheme.Surface.secondary,
                in: RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.medium, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.medium, style: .continuous)
                    .stroke(GamePlatformTheme.Surface.separator.opacity(0.5), lineWidth: 1)
            }
            .onChange(of: configuration.isPressed) { pressed in
                onPressChanged(pressed)
            }
    }
}
