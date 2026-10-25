import SwiftUI

/// Aspect-fit renderer host for portrait/landscape game canvases. The caller
/// supplies safe-area insets when its parent intentionally extends edge-to-edge;
/// otherwise pass zero insets and let the normal SwiftUI safe area constrain it.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerAspectFitContainer<Renderer: View>: View {
    public let gameAspectRatio: Double
    public let safeAreaInsets: EdgeInsets
    public let background: Color
    private let renderer: Renderer

    public init(
        gameAspectRatio: Double,
        safeAreaInsets: EdgeInsets = EdgeInsets(),
        background: Color = .black,
        @ViewBuilder renderer: () -> Renderer
    ) {
        self.gameAspectRatio = gameAspectRatio
        self.safeAreaInsets = safeAreaInsets
        self.background = background
        self.renderer = renderer()
    }

    public var body: some View {
        GeometryReader { geometry in
            let rect = PlayerViewportPolicy.aspectFit(
                container: PlayerSize(width: geometry.size.width, height: geometry.size.height),
                safeArea: PlayerInsets(
                    top: safeAreaInsets.top,
                    leading: safeAreaInsets.leading,
                    bottom: safeAreaInsets.bottom,
                    trailing: safeAreaInsets.trailing
                ),
                gameAspectRatio: gameAspectRatio
            )

            ZStack {
                background
                if let rect {
                    renderer
                        .frame(width: rect.width, height: rect.height)
                        .position(x: rect.x + rect.width / 2, y: rect.y + rect.height / 2)
                        .clipped()
                } else {
                    ContentUnavailableView(
                        "Unsupported game size",
                        systemImage: "rectangle.expand.vertical",
                        description: Text("This game has an invalid aspect ratio or the player area is too small.")
                    )
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .background(background)
        .accessibilityElement(children: .contain)
    }
}
