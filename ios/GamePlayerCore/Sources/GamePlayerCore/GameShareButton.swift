import SwiftUI

/// Native iOS share-sheet affordance for version-pinned game links. Link creation
/// is delegated to the trusted-origin builder; recipients must still pass server
/// authorization when they open the link.
@available(iOS 16.0, macOS 13.0, *)
public struct GameShareButton: View {
    public let title: String
    public let shareURL: URL
    public let compact: Bool

    public init(
        title: String,
        lineage: GameShareLineage,
        builder: GameShareLinkBuilder,
        compact: Bool = false
    ) throws {
        self.title = title
        self.shareURL = try builder.makeLink(for: lineage)
        self.compact = compact
    }

    public var body: some View {
        ShareLink(
            item: shareURL,
            subject: Text(title),
            message: Text("Play “\(title)”"),
            preview: SharePreview(title, image: Image(systemName: "gamecontroller.fill"))
        ) {
            if compact {
                Image(systemName: "square.and.arrow.up")
                    .frame(minWidth: 44, minHeight: 44)
            } else {
                Label("Share", systemImage: "square.and.arrow.up")
                    .frame(minHeight: 44)
            }
        }
        .accessibilityLabel("Share \(title)")
        .accessibilityHint("Shares a link to this game")
    }
}

/// A compact, accessible attribution line for games created as remixes.
@available(iOS 16.0, macOS 13.0, *)
public struct RemixAttributionView: View {
    public let parentTitle: String
    public let onOpenParent: () -> Void

    public init(parentTitle: String, onOpenParent: @escaping () -> Void) {
        self.parentTitle = parentTitle
        self.onOpenParent = onOpenParent
    }

    public var body: some View {
        Button(action: onOpenParent) {
            Label("Remix of \(parentTitle)", systemImage: "arrow.triangle.branch")
                .font(.subheadline)
                .lineLimit(2)
                .frame(minHeight: 44, alignment: .leading)
        }
        .buttonStyle(.plain)
        .accessibilityHint("Opens the original game")
    }
}
