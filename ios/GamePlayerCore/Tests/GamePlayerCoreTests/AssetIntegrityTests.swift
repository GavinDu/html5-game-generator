import CryptoKit
import XCTest
@testable import GamePlayerCore

final class AssetIntegrityTests: XCTestCase {
    func testAcceptsCorrectSHA256() throws {
        let bytes = Data("verified game asset".utf8)
        let hash = SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
        XCTAssertNoThrow(try AssetIntegrity.verify(bytes, expectedSHA256: hash))
        XCTAssertNoThrow(try AssetIntegrity.verify(bytes, expectedSHA256: hash.uppercased()))
    }

    func testRejectsMissingMalformedOrMismatchedHash() throws {
        let bytes = Data("asset".utf8)
        XCTAssertThrowsError(try AssetIntegrity.verify(bytes, expectedSHA256: nil)) {
            XCTAssertEqual($0 as? AssetIntegrityError, .missingHash)
        }
        XCTAssertThrowsError(try AssetIntegrity.verify(bytes, expectedSHA256: "bad")) {
            XCTAssertEqual($0 as? AssetIntegrityError, .invalidHash)
        }
        XCTAssertThrowsError(try AssetIntegrity.verify(bytes, expectedSHA256: String(repeating: "0", count: 64))) {
            XCTAssertEqual($0 as? AssetIntegrityError, .mismatch)
        }
    }

    func testEnforcesSizeLimitAndTrustedOrigin() throws {
        XCTAssertThrowsError(try AssetIntegrity.verify(Data(repeating: 1, count: 5), expectedSHA256: String(repeating: "a", count: 64), maxBytes: 4)) {
            XCTAssertEqual($0 as? AssetIntegrityError, .tooLarge)
        }
        let origin = URL(string: "https://games.example.test")!
        XCTAssertTrue(AssetIntegrity.isTrustedAssetURL(URL(string: "https://games.example.test/v1/assets/a")!, apiOrigin: origin))
        XCTAssertFalse(AssetIntegrity.isTrustedAssetURL(URL(string: "https://cdn.example.test/a")!, apiOrigin: origin))
        XCTAssertFalse(AssetIntegrity.isTrustedAssetURL(URL(string: "http://games.example.test/a")!, apiOrigin: origin))
    }
}
