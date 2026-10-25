import SwiftUI

/// Shared tokens for the iOS creator, discovery, social, and player surfaces.
/// Keep these semantic and dynamic so light/dark appearance and Dynamic Type work.
@available(iOS 16.0, macOS 13.0, *)
public enum GameDesignTokens {
    public static let spacing2: CGFloat = 8
    public static let spacing3: CGFloat = 12
    public static let spacing4: CGFloat = 16
    public static let spacing6: CGFloat = 24
    public static let cardRadius: CGFloat = 18
    public static let controlRadius: CGFloat = 12
    public static let minimumTapTarget: CGFloat = 44
    public static let contentMaxWidth: CGFloat = 720

    public static let cardBackground = Color.primary.opacity(0.045)
    public static let secondaryBackground = Color.primary.opacity(0.075)
    public static let divider = Color.primary.opacity(0.12)
    public static let secondaryText = Color.secondary
}
