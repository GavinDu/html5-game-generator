import SwiftUI

/// Maps SwiftUI scene lifecycle into the player session. Returning to the app
/// only auto-resumes a game that was paused specifically because it backgrounded.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerSceneLifecycleModifier: ViewModifier {
    @Environment(\.scenePhase) private var scenePhase
    @ObservedObject private var session: PlayerSessionModel

    public init(session: PlayerSessionModel) {
        self.session = session
    }

    public func body(content: Content) -> some View {
        content
            .onChange(of: scenePhase) { phase in
                switch phase {
                case .background:
                    session.pauseForBackground()
                case .active:
                    session.resumeFromBackground()
                case .inactive:
                    break
                @unknown default:
                    break
                }
            }
    }
}

@available(iOS 16.0, macOS 13.0, *)
public extension View {
    func observePlayerSceneLifecycle(_ session: PlayerSessionModel) -> some View {
        modifier(PlayerSceneLifecycleModifier(session: session))
    }
}
