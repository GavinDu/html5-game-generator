import SwiftUI

/// Shared discovery/social-feed card. Actions are injected so the same visual
/// component can be reused by the home feed, creator preview, and remix picker.
@available(iOS 16.0, macOS 13.0, *)
public struct GameFeedCard: View {
    public let title: String
    public let creatorName: String
    public let description: String
    public let thumbnailURL: URL?
    public let gameVersion: String
    public let parentGameTitle: String?
    public let onPlay: () -> Void
    public let onRemix: () -> Void
    public let onShare: () -> Void

    public init(
        title: String,
        creatorName: String,
        description: String,
        thumbnailURL: URL? = nil,
        gameVersion: String,
        parentGameTitle: String? = nil,
        onPlay: @escaping () -> Void,
        onRemix: @escaping () -> Void,
        onShare: @escaping () -> Void
    ) {
        self.title = title
        self.creatorName = creatorName
        self.description = description
        self.thumbnailURL = thumbnailURL
        self.gameVersion = gameVersion
        self.parentGameTitle = parentGameTitle
        self.onPlay = onPlay
        self.onRemix = onRemix
        self.onShare = onShare
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: GamePlatformTheme.Spacing.medium) {
            ZStack {
                RoundedRectangle(cornerRadius: GamePlatformTheme.Radius.medium, style: .continuous)
                    .fill(GamePlatformTheme.Surface.elevated)
                if let thumbnailURL {
                    AsyncImage(url: thumbnailURL) { phase in
                        switch phase {
                        case .success(let image):
                            image.resizable().scaledToFill()
                        case .failure:
                            placeholder
                        case .empty:
                            ProgressView().accessibilityLabel("Loading game thumbnail")
                        @unknown default:
                            placeholder
                        }
                    }
                    .frame(maxWidth: .infinity)
                    .frame(height: 176)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .accessibilityLabel("\(title) game thumbnail")
                } else {
                    placeholder
                }
            }
            .frame(height: 176)
            .accessibilityHidden(thumbnailURL == nil)

            VStack(alignment: .leading, spacing: GamePlatformTheme.Spacing.small) {
                Text(title)
                    .font(.headline)
                    .lineLimit(2)
                    .accessibilityAddTraits(.isHeader)

                Text("by \(creatorName)")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)

                if let parentGameTitle {
                    Label("Remix of \(parentGameTitle)", systemImage: "arrow.triangle.branch")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .accessibilityLabel("Remix of \(parentGameTitle)")
                }

                if !description.isEmpty {
                    Text(description)
                        .font(.body)
                        .lineLimit(3)
                }

                Text("Version \(gameVersion)")
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }

            HStack(spacing: GamePlatformTheme.Spacing.small) {
                Button(action: onPlay) {
                    Label("Play", systemImage: "play.fill")
                        .frame(maxWidth: .infinity, minHeight: GamePlatformTheme.Control.minimumHitTarget)
                }
                .buttonStyle(.borderedProminent)
                .accessibilityHint("Opens \(title) in the game player")

                Button(action: onRemix) {
                    Label("Remix", systemImage: "arrow.triangle.branch")
                        .frame(minWidth: GamePlatformTheme.Control.minimumHitTarget, minHeight: GamePlatformTheme.Control.minimumHitTarget)
                }
                .buttonStyle(.bordered)
                .accessibilityHint("Create your own version of \(title)")

                Button(action: onShare) {
                    Image(systemName: "square.and.arrow.up")
                        .frame(minWidth: 44, minHeight: 44)
                }
                .buttonStyle(.bordered)
                .accessibilityLabel("Share \(title)")
            }
        }
        .gamePlatformCard(padding: GamePlatformTheme.Spacing.large)
        .accessibilityElement(children: .contain)
    }

    private var placeholder: some View {
        VStack(spacing: 8) {
            Image(systemName: "gamecontroller.fill")
                .font(.largeTitle)
            Text("Game preview")
                .font(.caption)
        }
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityLabel("Game thumbnail unavailable")
    }
}
