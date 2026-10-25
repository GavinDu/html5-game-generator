import Foundation

/// Platform-independent policy for audio interruptions. The app's AVAudioSession
/// notification observer should forward interruption begin/end events here.
@MainActor
public final class PlayerInterruptionController {
    private let session: PlayerSessionModel
    private let pauseRuntime: () -> Void
    private let resumeRuntime: () -> Void
    private var wasPlayingBeforeInterruption = false

    public init(
        session: PlayerSessionModel,
        pauseRuntime: @escaping () -> Void,
        resumeRuntime: @escaping () -> Void
    ) {
        self.session = session
        self.pauseRuntime = pauseRuntime
        self.resumeRuntime = resumeRuntime
    }

    public func interruptionBegan() {
        guard case .playing = session.state else {
            wasPlayingBeforeInterruption = false
            return
        }
        wasPlayingBeforeInterruption = true
        session.pauseForInterruption()
        pauseRuntime()
    }

    /// shouldResume reflects system interruption options and app policy. A
    /// user-paused game is never auto-resumed by an interruption end.
    public func interruptionEnded(shouldResume: Bool) {
        let shouldResumePlayback = wasPlayingBeforeInterruption && shouldResume
        wasPlayingBeforeInterruption = false
        guard shouldResumePlayback,
              case .paused(_, _, .interruption) = session.state else { return }
        session.resume()
        resumeRuntime()
    }
}
