import CryptoKit
import Foundation

public enum PlayerLaunchStage: Equatable, Sendable {
    case idle
    case fetchingPackage
    case validatingPackage
    case loadingAssets(completed: Int, total: Int)
    case startingRuntime
    case running
    case paused
    case failed(PlayerFailure)
    case stopped
}

public enum PlayerLaunchError: Error, Equatable {
    case missingAssetHash(String)
    case assetHashMismatch(String)
    case duplicateAssetID(String)
}

public protocol GameRuntimeHost: Sendable {
    func start(package: GamePackageEnvelope, assets: [String: Data]) async throws
    func pause() async
    func resume() async throws
    func stop() async
}

/// Orchestrates the launch sequence without binding the core to URLSession,
/// SwiftUI, or a particular WKWebView renderer. All package and asset checks
/// finish before the runtime is allowed to start.
public actor PlayerLaunchCoordinator {
    public typealias StageHandler = @Sendable (PlayerLaunchStage) -> Void

    private let packageTransport: any GamePackageTransport
    private let assetTransport: any GameAssetTransport
    private let runtime: any GameRuntimeHost
    private let contract: PlayerContract
    private let onStageChange: StageHandler
    private var currentStage: PlayerLaunchStage = .idle
    private var activeTask: Task<Void, Error>?
    private var activeLaunchID: UUID?

    public init(
        packageTransport: any GamePackageTransport,
        assetTransport: any GameAssetTransport,
        runtime: any GameRuntimeHost,
        contract: PlayerContract = PlayerContract(),
        onStageChange: @escaping StageHandler = { _ in }
    ) {
        self.packageTransport = packageTransport
        self.assetTransport = assetTransport
        self.runtime = runtime
        self.contract = contract
        self.onStageChange = onStageChange
    }

    public var stage: PlayerLaunchStage { currentStage }

    public func launch(gameID: String, version: String? = nil) async throws {
        let launchID = UUID()
        activeTask?.cancel()
        activeTask = nil
        activeLaunchID = launchID
        await runtime.stop()
        guard activeLaunchID == launchID else { throw PlayerFailure.cancelled }

        let task = Task { try await self.performLaunch(gameID: gameID, version: version, launchID: launchID) }
        activeTask = task
        defer {
            if activeLaunchID == launchID {
                activeTask = nil
                activeLaunchID = nil
            }
        }
        try await task.value
    }

    public func pause() async {
        guard currentStage == .running else { return }
        await runtime.pause()
        transition(to: .paused)
    }

    public func resume() async throws {
        guard currentStage == .paused else { return }
        try await runtime.resume()
        transition(to: .running)
    }

    public func stop() async {
        activeLaunchID = nil
        activeTask?.cancel()
        activeTask = nil
        await runtime.stop()
        transition(to: .stopped)
    }

    private func performLaunch(gameID: String, version: String?, launchID: UUID) async throws {
        do {
            try Task.checkCancellation()
            transition(to: .fetchingPackage, launchID: launchID)
            let bytes = try await packageTransport.fetchPackage(gameID: gameID, version: version)
            try Task.checkCancellation()
            transition(to: .validatingPackage, launchID: launchID)
            let package = try contract.validate(bytes, expectedGameID: gameID)

            var assets = [String: Data]()
            var seenIDs = Set<String>()
            let total = package.assets.count
            for (index, asset) in package.assets.enumerated() {
                try Task.checkCancellation()
                guard seenIDs.insert(asset.id).inserted else {
                    throw PlayerLaunchError.duplicateAssetID(asset.id)
                }
                guard let expectedHash = asset.sha256, expectedHash.count == 64 else {
                    throw PlayerLaunchError.missingAssetHash(asset.id)
                }
                let assetBytes = try await assetTransport.fetchAsset(asset)
                try Task.checkCancellation()
                let actualHash = SHA256.hash(data: assetBytes).map { String(format: "%02x", $0) }.joined()
                guard actualHash.caseInsensitiveCompare(expectedHash) == .orderedSame else {
                    throw PlayerLaunchError.assetHashMismatch(asset.id)
                }
                assets[asset.id] = assetBytes
                transition(to: .loadingAssets(completed: index + 1, total: total), launchID: launchID)
            }

            try Task.checkCancellation()
            transition(to: .startingRuntime, launchID: launchID)
            try await runtime.start(package: package, assets: assets)
            try Task.checkCancellation()
            transition(to: .running)
        } catch is CancellationError {
            transition(to: .failed(.cancelled), launchID: launchID)
            throw PlayerFailure.cancelled
        } catch let failure as PlayerFailure {
            transition(to: .failed(failure), launchID: launchID)
            throw failure
        } catch let failure as PlayerLaunchError {
            transition(to: .failed(.malformedPackage), launchID: launchID)
            throw failure
        } catch {
            transition(to: .failed(.network(String(describing: error))), launchID: launchID)
            throw error
        }
    }

    private func transition(to stage: PlayerLaunchStage, launchID: UUID? = nil) {
        if let launchID, activeLaunchID != launchID { return }
        currentStage = stage
        onStageChange(stage)
    }
}
