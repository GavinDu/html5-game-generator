import SwiftUI

/// Shared semantic tokens for discovery, creator, and player screens. Prefer
/// semantic roles over one-off colors so light/dark appearance and contrast
/// remain consistent across the app.
@available(iOS 16.0, macOS 13.0, *)
public enum GamePlatformTheme {
    public enum Spacing {
        public static let xSmall: CGFloat = 4
        public static let small: CGFloat = 8
        public static let medium: CGFloat = 12
        public static let large: CGFloat = 16
        public static let xLarge: CGFloat = 24
        public static let xxLarge: CGFloat = 32
    }

    public enum Radius {
        public static let small: CGFloat = 8
        public static let medium: CGFloat = 12
        public static let large: CGFloat = 18
        public static let pill: CGFloat = 999
    }

    public enum Control {
        public static let minimumHitTarget: CGFloat = 44
        public static let minimumReadableText: CGFloat = 11
    }

    public enum Surface {
        public static let canvas = Color.primary.opacity(0.02)
        public static let secondary = Color.primary.opacity(0.045)
        public static let elevated = Color.primary.opacity(0.07)
        public static let separator = Color.primary.opacity(0.16)
    }

    public enum Feedback {
        public static let success = Color.green
        public static let warning = Color.orange
        public static let danger = Color.red
        public static let accent = Color.accentColor
    }
}

@available(iOS 16.0, macOS 13.0, *)
public struct GamePlatformCardStyle: ViewModifier {
    public var padding: CGFloat
    public init(padding: CGFloat = GamePlatformTheme.Spacing.large) {
        self.padding = padding
    }

    public func body(content: Content) -> some View {
        content
            .padding(padding)
            .background(GamePlatformTheme.Surface.secondary)
            .clipShape(RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.large, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.large, style: .continuous)
                    .stroke(GamePlatformTheme.Surface.separator.opacity(0.45), lineWidth: 1)
            }
    }
}

@available(iOS 16.0, macOS 13.0, *)
public extension View {
    func gamePlatformCard(padding: CGFloat = GamePlatformTheme.Spacing.large) -> some View {
        modifier(GamePlatformCardStyle(padding: padding))
    }
}
