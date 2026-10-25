import SwiftUI

/// Binds the observable session model to the shared player shell. The host still
/// owns networking and the runtime bridge, passed through the action closures.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerSessionView<Renderer: View>: View {
    @Environment(\.scenePhase) private var scenePhase
    @ObservedObject private var session: PlayerSessionModel
    private let renderer: Renderer
    private let onStartRuntime: () -> Void
    private let onPauseRuntime: () -> Void
    private let onResumeRuntime: () -> Void
    private let onRetryLoad: () -> Void
    private let onExitPlayer: () -> Void
    private let onRestartGame: () -> Void

    public init(
        session: PlayerSessionModel,
        @ViewBuilder renderer: () -> Renderer,
        onStartRuntime: @escaping () -> Void,
        onPauseRuntime: @escaping () -> Void,
        onResumeRuntime: @escaping () -> Void,
        onRetryLoad: @escaping () -> Void,
        onExitPlayer: @escaping () -> Void,
        onRestartGame: @escaping () -> Void
    ) {
        self.session = session
        self.renderer = renderer()
        self.onStartRuntime = onStartRuntime
        self.onPauseRuntime = onPauseRuntime
        self.onResumeRuntime = onResumeRuntime
        self.onRetryLoad = onRetryLoad
        self.onExitPlayer = onExitPlayer
        self.onRestartGame = onRestartGame
    }

    public var body: some View {
        PlayerExperienceView(
            state: session.state,
            title: session.gameTitle.isEmpty ? "Game" : session.gameTitle,
            progress: session.loadProgress,
            renderer: { renderer },
            onPlay: {
                session.start()
                onStartRuntime()
            },
            onPause: {
                session.pauseByUser()
                onPauseRuntime()
            },
            onResume: {
                session.resume()
                onResumeRuntime()
            },
            onRetry: {
                session.retry()
                onRetryLoad()
            },
            onExit: onExitPlayer,
            onRestart: {
                session.reset()
                onRestartGame()
            }
        )
        .onChange(of: scenePhase) { phase in
            if phase == .background {
                if case .playing = session.state {
                    session.pauseForBackground()
                    onPauseRuntime()
                }
            } else if phase == .active {
                if case .paused(_, _, .appBackgrounded) = session.state {
                    session.resume()
                    onResumeRuntime()
                }
            }
        }
    }
}
