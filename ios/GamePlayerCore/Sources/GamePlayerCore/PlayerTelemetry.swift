import Foundation

/// Privacy-minimal player telemetry. Events intentionally exclude usernames,
/// device identifiers, free-form error messages, and asset URLs.
public struct PlayerTelemetryEvent: Equatable, Sendable {
    public enum Name: String, Equatable, Sendable {
        case loadStarted
        case packageReady
        case playStarted
        case paused
        case resumed
        case loadFailed
        case retryRequested
        case gameCompleted
        case sessionReset
    }

    public let name: Name
    public let gameID: String?
    public let packageVersion: String?
    public let failureCategory: String?

    public init(
        name: Name,
        gameID: String? = nil,
        packageVersion: String? = nil,
        failureCategory: String? = nil
    ) {
        self.name = name
        self.gameID = gameID
        self.packageVersion = packageVersion
        self.failureCategory = failureCategory
    }

    public static func failureName(for failure: PlayerFailure) -> String {
        switch failure {
        case .invalidURL: return "invalid_url"
        case .network: return "network"
        case .httpStatus: return "http_status"
        case .unsupportedPackageFormat: return "unsupported_package_format"
        case .unsupportedRuntime: return "unsupported_runtime"
        case .packageGameIDMismatch: return "game_id_mismatch"
        case .missingIR: return "missing_ir"
        case .invalidIRHash: return "invalid_ir_hash"
        case .malformedPackage: return "malformed_package"
        case .cancelled: return "cancelled"
        }
    }
}
