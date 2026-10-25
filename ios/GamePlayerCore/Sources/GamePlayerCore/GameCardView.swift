import SwiftUI

@available(iOS 16.0, macOS 13.0, *)
public struct GameCardModel: Identifiable, Equatable, Sendable {
    public enum Visibility: String, Sendable {
        case publicGame
        case privateGame
        case processing
        case quarantined
    }

    public let id: String
    public let title: String
    public let creatorName: String
    public let subtitle: String?
    public let visibility: Visibility
    public let remixParentID: String?

    public init(id: String, title: String, creatorName: String, subtitle: String? = nil,
                visibility: Visibility = .publicGame, remixParentID: String? = nil) {
        self.id = id
        self.title = title
        self.creatorName = creatorName
        self.subtitle = subtitle
        self.visibility = visibility
        self.remixParentID = remixParentID
    }
}

/// Shared discovery/social card with visible attribution and consistent actions.
/// The preview is injected so callers can use a thumbnail, snapshot, or live preview.
@available(iOS 16.0, macOS 13.0, *)
public struct GameCardView<Preview: View>: View {
    public let game: GameCardModel
    public let preview: Preview
    public let onPlay: () -> Void
    public let onShare: () -> Void
    public let onRemix: () -> Void

    public init(game: GameCardModel, @ViewBuilder preview: () -> Preview,
                onPlay: @escaping () -> Void, onShare: @escaping () -> Void,
                onRemix: @escaping () -> Void) {
        self.game = game
        self.preview = preview()
        self.onPlay = onPlay
        self.onShare = onShare
        self.onRemix = onRemix
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: GameDesignTokens.spacing3) {
            ZStack(alignment: .bottomLeading) {
                preview
                    .frame(maxWidth: .infinity)
                    .aspectRatio(16 / 9, contentMode: .fit)
                    .clipped()
                    .accessibilityHidden(true)
                Label(visibilityTitle, systemImage: visibilitySymbol)
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(.ultraThinMaterial, in: Capsule())
                    .padding(10)
                    .accessibilityLabel("Visibility: \(visibilityTitle)")
            }

            VStack(alignment: .leading, spacing: 4) {
                Text(game.title)
                    .font(.headline)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                Text("By \(game.creatorName)")
                    .font(.subheadline)
                    .foregroundStyle(GameDesignTokens.secondaryText)
                if let subtitle = game.subtitle, !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.subheadline)
                        .foregroundStyle(GameDesignTokens.secondaryText)
                        .lineLimit(3)
                }
                if game.remixParentID != nil {
                    Label("Remix", systemImage: "arrow.triangle.branch")
                        .font(.caption)
                        .foregroundStyle(GameDesignTokens.secondaryText)
                }
            }
            .padding(.horizontal, GameDesignTokens.spacing3)

            HStack(spacing: GameDesignTokens.spacing2) {
                Button(action: onPlay) {
                    Label("Play", systemImage: "play.fill")
                        .frame(maxWidth: .infinity, minHeight: GameDesignTokens.minimumTapTarget)
                }
                .buttonStyle(.borderedProminent)
                .accessibilityHint("Opens this game in the player")

                Button(action: onShare) {
                    Image(systemName: "square.and.arrow.up")
                        .frame(minWidth: GameDesignTokens.minimumTapTarget,
                               minHeight: GameDesignTokens.minimumTapTarget)
                }
                .buttonStyle(.bordered)
                .accessibilityLabel("Share \(game.title)")

                Button(action: onRemix) {
                    Image(systemName: "arrow.triangle.branch")
                        .frame(minWidth: GameDesignTokens.minimumTapTarget,
                               minHeight: GameDesignTokens.minimumTapTarget)
                }
                .buttonStyle(.bordered)
                .accessibilityLabel("Remix \(game.title)")
            }
            .padding(.horizontal, GameDesignTokens.spacing3)
            .padding(.bottom, GameDesignTokens.spacing3)
        }
        .background(GameDesignTokens.cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: GameDesignTokens.cardRadius, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: GameDesignTokens.cardRadius, style: .continuous)
                .stroke(GameDesignTokens.divider, lineWidth: 1)
        }
        .frame(maxWidth: GameDesignTokens.contentMaxWidth)
    }

    private var visibilityTitle: String {
        switch game.visibility {
        case .publicGame: return "Public"
        case .privateGame: return "Private"
        case .processing: return "Processing"
        case .quarantined: return "Unavailable"
        }
    }

    private var visibilitySymbol: String {
        switch game.visibility {
        case .publicGame: return "globe"
        case .privateGame: return "lock.fill"
        case .processing: return "clock"
        case .quarantined: return "exclamationmark.shield"
        }
    }
}
