import Foundation

/// The states exposed to SwiftUI and UI automation. Every transition is explicit so
/// interruption recovery and retry behavior can be tested without a real device.
public enum PlayerState: Equatable, Sendable {
    case idle
    case loading
    case ready(gameID: String, packageVersion: String)
    case playing(gameID: String, packageVersion: String)
    case paused(gameID: String, packageVersion: String, reason: PauseReason)
    case failed(PlayerFailure)
    case completed(gameID: String, packageVersion: String)

    public enum PauseReason: String, Equatable, Sendable {
        case appBackgrounded
        case interruption
        case user
    }
}

public enum PlayerFailure: Error, Equatable, Sendable {
    case invalidURL
    case network(String)
    case httpStatus(Int)
    case unsupportedPackageFormat(String)
    case unsupportedRuntime(required: String, current: String)
    case packageGameIDMismatch
    case missingIR
    case invalidIRHash
    case assetHashMissing
    case assetHashMismatch
    case assetTooLarge
    case assetOriginMismatch
    case malformedPackage
    case cancelled
}

public struct PackageAsset: Decodable, Equatable, Sendable {
    public let id: String
    public let kind: String
    public let uri: String
    public let url: String
    public let mimeType: String?
    public let sha256: String?

    public init(id: String, kind: String, uri: String, url: String, mimeType: String? = nil, sha256: String? = nil) {
        self.id = id
        self.kind = kind
        self.uri = uri
        self.url = url
        self.mimeType = mimeType
        self.sha256 = sha256
    }
}

public struct GamePackageEnvelope: Decodable, Sendable {
    public let packageFormatVersion: String
    public let minRuntimeVersion: String
    public let gameId: String
    public let ir: JSONValue?
    public let assets: [PackageAsset]
    public let irHash: String
}

public indirect enum JSONValue: Decodable, Sendable {
    case object([String: JSONValue])
    case array([JSONValue])
    case string(String)
    case number(Double)
    case bool(Bool)
    case null

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let v = try? c.decode(Bool.self) { self = .bool(v); return }
        if let v = try? c.decode(Double.self) { self = .number(v); return }
        if let v = try? c.decode(String.self) { self = .string(v); return }
        if let v = try? c.decode([String: JSONValue].self) { self = .object(v); return }
        if let v = try? c.decode([JSONValue].self) { self = .array(v); return }
        throw DecodingError.dataCorruptedError(in: c, debugDescription: "Unsupported JSON value")
    }
}

public struct PlayerContract: Sendable {
    public let packageFormatVersion: String
    public let runtimeVersion: String

    public init(packageFormatVersion: String = "2.0", runtimeVersion: String = "2026.10.23.2") {
        self.packageFormatVersion = packageFormatVersion
        self.runtimeVersion = runtimeVersion
    }

    public func validate(_ data: Data, expectedGameID: String) throws -> GamePackageEnvelope {
        let envelope: GamePackageEnvelope
        do {
            envelope = try JSONDecoder().decode(GamePackageEnvelope.self, from: data)
        } catch {
            throw PlayerFailure.malformedPackage
        }
        guard envelope.packageFormatVersion == packageFormatVersion else {
            throw PlayerFailure.unsupportedPackageFormat(envelope.packageFormatVersion)
        }
        guard envelope.gameId == expectedGameID else { throw PlayerFailure.packageGameIDMismatch }
        guard envelope.minRuntimeVersion.compare(runtimeVersion, options: .numeric) != .orderedDescending else {
            throw PlayerFailure.unsupportedRuntime(required: envelope.minRuntimeVersion, current: runtimeVersion)
        }
        guard envelope.ir != nil else { throw PlayerFailure.missingIR }
        guard envelope.irHash.count == 64, envelope.irHash.allSatisfy({ $0.isHexDigit }) else {
            throw PlayerFailure.invalidIRHash
        }
        for asset in envelope.assets {
            guard !asset.id.isEmpty, !asset.uri.isEmpty, !asset.url.isEmpty else {
                throw PlayerFailure.malformedPackage
            }
            if let hash = asset.sha256, (hash.count != 64 || !hash.allSatisfy({ $0.isHexDigit })) {
                throw PlayerFailure.malformedPackage
            }
        }
        return envelope
    }
}

/// Pure lifecycle transition model. The host app can map scenePhase/audio
/// interruptions into these events, and the view can render each state.
public struct PlayerLifecycle: Sendable {
    public private(set) var state: PlayerState = .idle
    public private(set) var activeGame: (id: String, version: String)?

    public init() {}

    public mutating func beginLoad() { state = .loading }

    public mutating func packageReady(gameID: String, version: String) {
        activeGame = (gameID, version)
        state = .ready(gameID: gameID, packageVersion: version)
    }

    public mutating func start() {
        guard let game = activeGame else { return }
        state = .playing(gameID: game.id, packageVersion: game.version)
    }

    public mutating func pause(_ reason: PlayerState.PauseReason) {
        guard let game = activeGame else { return }
        switch state {
        case .playing, .ready:
            state = .paused(gameID: game.id, packageVersion: game.version, reason: reason)
        default:
            break
        }
    }

    public mutating func resume() {
        guard let game = activeGame else { return }
        guard case .paused = state else { return }
        state = .playing(gameID: game.id, packageVersion: game.version)
    }

    public mutating func fail(_ failure: PlayerFailure) { state = .failed(failure) }

    public mutating func retry() {
        guard case .failed = state else { return }
        activeGame = nil
        state = .loading
    }

    public mutating func complete() {
        guard let game = activeGame else { return }
        state = .completed(gameID: game.id, packageVersion: game.version)
    }

    public mutating func reset() {
        activeGame = nil
        state = .idle
    }
}
