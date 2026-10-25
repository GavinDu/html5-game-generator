import Combine
import Foundation

/// Main-thread observable adapter for SwiftUI. Networking/runtime code should
/// call these methods as the package load and renderer lifecycle progresses.
@MainActor
public final class PlayerSessionModel: ObservableObject {
    @Published public private(set) var state: PlayerState = .idle
    @Published public private(set) var loadProgress: Double?
    @Published public private(set) var gameTitle: String = ""

    private var lifecycle = PlayerLifecycle()

    public init() {}

    public func beginLoad(title: String) {
        gameTitle = title
        lifecycle.beginLoad()
        state = lifecycle.state
        loadProgress = 0
    }

    public func updateLoadProgress(_ progress: Double) {
        guard state == .loading else { return }
        loadProgress = min(max(progress, 0), 1)
    }

    public func packageReady(gameID: String, version: String) {
        lifecycle.packageReady(gameID: gameID, version: version)
        state = lifecycle.state
        loadProgress = 1
    }

    public func start() {
        lifecycle.start()
        state = lifecycle.state
    }

    public func pauseForBackground() {
        lifecycle.pause(.appBackgrounded)
        state = lifecycle.state
    }

    public func pauseForInterruption() {
        lifecycle.pause(.interruption)
        state = lifecycle.state
    }

    public func pauseByUser() {
        lifecycle.pause(.user)
        state = lifecycle.state
    }

    public func resume() {
        lifecycle.resume()
        state = lifecycle.state
    }

    public func fail(_ failure: PlayerFailure) {
        lifecycle.fail(failure)
        state = lifecycle.state
        loadProgress = nil
    }

    public func retry() {
        lifecycle.retry()
        state = lifecycle.state
        loadProgress = 0
    }

    public func complete() {
        lifecycle.complete()
        state = lifecycle.state
        loadProgress = nil
    }

    public func reset() {
        lifecycle.reset()
        state = lifecycle.state
        loadProgress = nil
        gameTitle = ""
    }
}
