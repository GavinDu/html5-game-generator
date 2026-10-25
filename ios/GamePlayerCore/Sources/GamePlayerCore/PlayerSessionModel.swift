import Combine
import Foundation

/// Main-thread observable adapter for SwiftUI. Networking/runtime code should
/// call these methods as the package load and renderer lifecycle progresses.
@MainActor
public final class PlayerSessionModel: ObservableObject {
    @Published public private(set) var state: PlayerState = .idle
    @Published public private(set) var loadProgress: Double?
    @Published public private(set) var gameTitle: String = ""

    /// Inject the app's telemetry sink. The model never sends data itself.
    public var onTelemetry: ((PlayerTelemetryEvent) -> Void)?

    private var lifecycle = PlayerLifecycle()
    private var activeGameID: String?
    private var activePackageVersion: String?

    public init(onTelemetry: ((PlayerTelemetryEvent) -> Void)? = nil) {
        self.onTelemetry = onTelemetry
    }

    public func beginLoad(title: String) {
        gameTitle = title
        lifecycle.beginLoad()
        state = lifecycle.state
        loadProgress = 0
        emit(.loadStarted)
    }

    public func updateLoadProgress(_ progress: Double) {
        guard state == .loading else { return }
        loadProgress = min(max(progress, 0), 1)
    }

    public func packageReady(gameID: String, version: String) {
        lifecycle.packageReady(gameID: gameID, version: version)
        activeGameID = gameID
        activePackageVersion = version
        state = lifecycle.state
        loadProgress = 1
        emit(.packageReady)
    }

    public func start() {
        lifecycle.start()
        state = lifecycle.state
        if case .playing = state { emit(.playStarted) }
    }

    public func pauseForBackground() {
        lifecycle.pause(.appBackgrounded)
        state = lifecycle.state
        if case .paused(_, _, let reason) = state { emit(.paused, failureCategory: reason.rawValue) }
    }

    public func pauseForInterruption() {
        lifecycle.pause(.interruption)
        state = lifecycle.state
        if case .paused(_, _, let reason) = state { emit(.paused, failureCategory: reason.rawValue) }
    }

    public func pauseByUser() {
        lifecycle.pause(.user)
        state = lifecycle.state
        if case .paused(_, _, let reason) = state { emit(.paused, failureCategory: reason.rawValue) }
    }

    public func resume() {
        lifecycle.resume()
        state = lifecycle.state
        if case .playing = state { emit(.resumed) }
    }

    public func fail(_ failure: PlayerFailure) {
        lifecycle.fail(failure)
        state = lifecycle.state
        loadProgress = nil
        emit(.loadFailed, failureCategory: PlayerTelemetryEvent.failureName(for: failure))
    }

    public func retry() {
        lifecycle.retry()
        state = lifecycle.state
        loadProgress = 0
        activeGameID = nil
        activePackageVersion = nil
        emit(.retryRequested)
    }

    public func complete() {
        lifecycle.complete()
        state = lifecycle.state
        loadProgress = nil
        emit(.gameCompleted)
    }

    public func reset() {
        lifecycle.reset()
        state = lifecycle.state
        loadProgress = nil
        gameTitle = ""
        activeGameID = nil
        activePackageVersion = nil
        emit(.sessionReset)
    }

    private func emit(_ name: PlayerTelemetryEvent.Name, failureCategory: String? = nil) {
        onTelemetry?(PlayerTelemetryEvent(
            name: name,
            gameID: activeGameID,
            packageVersion: activePackageVersion,
            failureCategory: failureCategory
        ))
    }
}
