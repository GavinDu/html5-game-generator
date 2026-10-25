import SwiftUI

/// Maps SwiftUI scene transitions into the player session lifecycle. Background
/// resume is automatic only for a pause caused by backgrounding; user pauses and
/// audio interruptions remain paused until the user explicitly resumes.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerSceneLifecycleBridge<Content: View>: View {
    @Environment(\.scenePhase) private var scenePhase
    @ObservedObject private var session: PlayerSessionModel
    private let content: Content
    private let pauseRuntime: () -> Void
    private let resumeRuntime: () -> Void

    public init(
        session: PlayerSessionModel,
        @ViewBuilder content: () -> Content,
        pauseRuntime: @escaping () -> Void,
        resumeRuntime: @escaping () -> Void
    ) {
        self.session = session
        self.content = content()
        self.pauseRuntime = pauseRuntime
        self.resumeRuntime = resumeRuntime
    }

    public var body: some View {
        content
            .onChange(of: scenePhase) { phase in
                if phase != .active {
                    switch session.state {
                    case .playing, .ready:
                        session.pauseForBackground()
                        pauseRuntime()
                    default:
                        break
                    }
                } else {
                    guard case .paused(_, _, .appBackgrounded) = session.state else { return }
                    session.resume()
                    resumeRuntime()
                }
            }
    }
}
