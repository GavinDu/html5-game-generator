import CryptoKit
import Foundation

public enum AssetIntegrityError: Error, Equatable, Sendable {
    case missingHash
    case invalidHash
    case mismatch
    case tooLarge
    case untrustedOrigin
}

/// Verifies asset bytes before they are passed to a game renderer.
public enum AssetIntegrity {
    public static func verify(
        _ data: Data,
        expectedSHA256: String?,
        maxBytes: Int = 20_000_000
    ) throws {
        guard let expectedSHA256, !expectedSHA256.isEmpty else {
            throw AssetIntegrityError.missingHash
        }
        guard expectedSHA256.count == 64, expectedSHA256.allSatisfy(\.isHexDigit) else {
            throw AssetIntegrityError.invalidHash
        }
        guard maxBytes > 0, data.count <= maxBytes else {
            throw AssetIntegrityError.tooLarge
        }
        let actual = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        guard actual.caseInsensitiveCompare(expectedSHA256) == .orderedSame else {
            throw AssetIntegrityError.mismatch
        }
    }

    public static func isTrustedAssetURL(_ assetURL: URL, apiOrigin: URL) -> Bool {
        guard assetURL.scheme?.lowercased() == "https",
              apiOrigin.scheme?.lowercased() == "https",
              assetURL.user == nil,
              assetURL.password == nil,
              apiOrigin.user == nil,
              apiOrigin.password == nil else {
            return false
        }
        return assetURL.host?.lowercased() == apiOrigin.host?.lowercased()
            && effectivePort(assetURL) == effectivePort(apiOrigin)
    }

    private static func effectivePort(_ url: URL) -> Int? {
        if let port = url.port { return port }
        return url.scheme?.lowercased() == "https" ? 443 : nil
    }
}
