import Foundation

/// Fetches the package envelope for a game, optionally pinned to a version.
public protocol GamePackageTransport: Sendable {
    func fetchPackage(gameID: String, version: String?) async throws -> Data
}

/// Fetches one asset from the trusted API origin. The coordinator verifies the
/// returned bytes against the package's SHA-256 before starting the runtime.
public protocol GameAssetTransport: Sendable {
    func fetchAsset(_ asset: PackageAsset) async throws -> Data
}
