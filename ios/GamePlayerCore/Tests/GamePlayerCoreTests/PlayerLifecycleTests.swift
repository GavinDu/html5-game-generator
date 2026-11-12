import CryptoKit
import XCTest
@testable import GamePlayerCore

final class PlayerLifecycleTests: XCTestCase {
    func testBackgroundPauseAndForegroundResumePreserveGameVersion() {
        var player = PlayerLifecycle()
        player.beginLoad()
        player.packageReady(gameID: "game-123", version: "v7")
        player.start()
        player.pause(.appBackgrounded)
        XCTAssertEqual(player.state, .paused(gameID: "game-123", packageVersion: "v7", reason: .appBackgrounded))
        player.resume()
        XCTAssertEqual(player.state, .playing(gameID: "game-123", packageVersion: "v7"))
    }

    func testInterruptionCanResumeAndUserPauseIsDistinct() {
        var player = PlayerLifecycle()
        player.packageReady(gameID: "g", version: "v1")
        player.start()
        player.pause(.interruption)
        XCTAssertEqual(player.state, .paused(gameID: "g", packageVersion: "v1", reason: .interruption))
        player.resume()
        player.pause(.user)
        XCTAssertEqual(player.state, .paused(gameID: "g", packageVersion: "v1", reason: .user))
    }

    func testFailedLoadCanRetryWithoutReusingStaleGame() {
        var player = PlayerLifecycle()
        player.beginLoad()
        player.fail(.network("offline"))
        XCTAssertEqual(player.state, .failed(.network("offline")))
        player.retry()
        XCTAssertEqual(player.state, .loading)
        player.start()
        XCTAssertEqual(player.state, .loading)
    }

    func testCompletionAndReset() {
        var player = PlayerLifecycle()
        player.packageReady(gameID: "g", version: "v2")
        player.start()
        player.complete()
        XCTAssertEqual(player.state, .completed(gameID: "g", packageVersion: "v2"))
        player.reset()
        XCTAssertEqual(player.state, .idle)
    }
}

final class PlayerContractTests: XCTestCase {
    private func package(
        format: String = "2.0",
        runtime: String = "2026.10.23.2",
        gameID: String = "game-123",
        irVersion: String = "1.0",
        ir: String = #"{"irVersion":"1.0"}"#,
        irHash: String? = nil,
        irSha256: String? = nil,
        assets: String = "[]"
    ) -> Data {
        let calculatedDigest = SHA256.hash(data: Data(ir.utf8)).map { String(format: "%02x", $0) }.joined()
        let fullHash = irSha256 ?? calculatedDigest
        let boardHash = irHash ?? String(fullHash.prefix(12))
        return Data("""
        {
          "packageFormatVersion":"\(format)",
          "minRuntimeVersion":"\(runtime)",
          "irVersion":"\(irVersion)",
          "gameId":"\(gameID)",
          "ir":\(ir),
          "assets":\(assets),
          "irHash":"\(boardHash)",
          "irSha256":"\(fullHash)"
        }
        """.utf8)
    }

    func testAcceptsSupportedPackageEnvelope() throws {
        let result = try PlayerContract().validate(package(), expectedGameID: "game-123")
        XCTAssertEqual(result.gameId, "game-123")
        XCTAssertEqual(result.irVersion, "1.0")
        XCTAssertEqual(result.assets.count, 0)
        XCTAssertEqual(result.irHash.count, 12)
        XCTAssertEqual(result.irHash, String(result.irSha256.prefix(12)))
    }

    /// CI sets this path to a real JSON envelope emitted by the Go delivery
    /// handler. Local Swift tests still exercise the exact-byte hashing unit
    /// fixtures when Go is not available.
    func testAcceptsBackendGeneratedCanonicalPackage() throws {
        guard let fixturePath = ProcessInfo.processInfo.environment["GAME_PACKAGE_FIXTURE_PATH"] else {
            return
        }
        let data = try Data(contentsOf: URL(fileURLWithPath: fixturePath))
        let backendEnvelope = try JSONDecoder().decode(GamePackageEnvelope.self, from: data)
        let accepted = try PlayerContract().validate(data, expectedGameID: backendEnvelope.gameId)
        XCTAssertEqual(accepted.irHash.count, 12)
        XCTAssertEqual(accepted.irSha256.count, 64)
        XCTAssertEqual(accepted.irHash, String(accepted.irSha256.prefix(12)))
    }

    func testRejectsUnsupportedPackageFormat() {
        XCTAssertThrowsError(try PlayerContract().validate(package(format: "99.0"), expectedGameID: "game-123")) { error in
            XCTAssertEqual(error as? PlayerFailure, .unsupportedPackageFormat("99.0"))
        }
    }

    func testRejectsNewerMinimumRuntime() {
        XCTAssertThrowsError(try PlayerContract().validate(package(runtime: "2027.1"), expectedGameID: "game-123")) { error in
            XCTAssertEqual(error as? PlayerFailure, .unsupportedRuntime(required: "2027.1", current: "2026.10.23.2"))
        }
    }

    func testRejectsWrongGameIDMissingIRAndInvalidHash() {
        XCTAssertThrowsError(try PlayerContract().validate(package(), expectedGameID: "other"))
        XCTAssertThrowsError(try PlayerContract().validate(package(ir: "null"), expectedGameID: "game-123"))
        XCTAssertThrowsError(try PlayerContract().validate(package(irHash: "not-a-hash"), expectedGameID: "game-123"))
    }

    func testRejectsHashMismatchEvenWhenBothIdentifiersLookValid() {
        let wrongFullDigest = String(repeating: "b", count: 64)
        XCTAssertThrowsError(
            try PlayerContract().validate(
                package(irSha256: wrongFullDigest), expectedGameID: "game-123"
            )
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .invalidIRHash)
        }
        XCTAssertThrowsError(
            try PlayerContract().validate(
                package(irHash: String(repeating: "f", count: 12)), expectedGameID: "game-123"
            )
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .invalidIRHash)
        }
    }

    func testRejectsTamperedIRPayloadWithoutRehashing() {
        let original = package(ir: #"{"irVersion":"1.0","game":{"title":"A"}}"#)
        let tampered = String(data: original, encoding: .utf8)!
            .replacingOccurrences(of: #""title":"A""#, with: #""title":"B""#)
        XCTAssertThrowsError(
            try PlayerContract().validate(Data(tampered.utf8), expectedGameID: "game-123")
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .invalidIRHash)
        }
    }

    func testRejectsDuplicateTopLevelIRField() {
        let original = package()
        let duplicate = String(data: original, encoding: .utf8)!
            .replacingOccurrences(
                of: #""ir":{"irVersion":"1.0"}"#,
                with: #""ir":{"irVersion":"1.0"},"ir":{"irVersion":"1.0"}"#
            )
        XCTAssertThrowsError(
            try PlayerContract().validate(Data(duplicate.utf8), expectedGameID: "game-123")
        ) { error in
            // Foundation decoder versions may reject duplicate keys before
            // our raw-field scanner sees them. Either rejection is safe.
            let failure = error as? PlayerFailure
            XCTAssertTrue(failure == .invalidIRHash || failure == .malformedPackage)
        }
    }

    func testNestedJSONAndEscapedTextAreHashedAsReceived() throws {
        let nested = #"{"irVersion":"1.0","game":{"title":"hello \"game\"","scenes":[{"id":"s","items":[1,2]}]}}"#
        let result = try PlayerContract().validate(package(ir: nested), expectedGameID: "game-123")
        XCTAssertEqual(result.irHash.count, 12)
    }

    func testAcceptsKnownIR12CapabilityManifest() throws {
        let ir = #"{"irVersion":"1.2","capabilities":["shell.v1","render.sprite"]}"#
        let result = try PlayerContract().validate(
            package(irVersion: "1.2", ir: ir), expectedGameID: "game-123"
        )
        XCTAssertEqual(result.irVersion, "1.2")
        XCTAssertEqual(PlayerContract.supportedCapabilities.count, 19)
    }

    func testCapabilityPreflightRejectsUnknownDuplicateOrUnboundedManifest() {
        let invalid = [
            #"{"irVersion":"1.0","capabilities":["render.unknown"]}"#,
            #"{"irVersion":"1.0","capabilities":["render.sprite","render.sprite"]}"#,
            #"{"irVersion":"1.0","capabilities":"render.sprite"}"#,
            #"{"irVersion":"1.0","capabilities":[1]}"#,
            #"{"irVersion":"1.2","capabilities":["render.sprite"]}"#,
            #"{"irVersion":"1.2"}"#,
            #"{"irVersion":"1.1","experience":{"archetype":"catcher"}}"#,
        ]
        for ir in invalid {
            let version = ir.contains(#""1.2""#) ? "1.2" : ir.contains(#""1.1""#) ? "1.1" : "1.0"
            XCTAssertThrowsError(
                try PlayerContract().validate(
                    package(irVersion: version, ir: ir), expectedGameID: "game-123"
                ), "invalid capabilities unexpectedly accepted: \(ir)"
            ) { error in
                XCTAssertEqual(error as? PlayerFailure, .unsupportedCapabilities)
            }
        }
        let oversized = Array(repeating: "\"render.sprite\"", count: 33).joined(separator: ",")
        let ir = "{\"irVersion\":\"1.0\",\"capabilities\":[\(oversized)]}"
        XCTAssertThrowsError(
            try PlayerContract().validate(package(ir: ir), expectedGameID: "game-123")
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .unsupportedCapabilities)
        }
    }

    func testRejectsUnsupportedIRVersion() {
        XCTAssertThrowsError(
            try PlayerContract().validate(
                package(irVersion: "9.0", ir: #"{"irVersion":"9.0"}"#),
                expectedGameID: "game-123"
            )
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .unsupportedIRVersion("9.0"))
        }
    }

    func testRejectsEnvelopeAndEmbeddedIRVersionMismatch() {
        XCTAssertThrowsError(
            try PlayerContract().validate(
                package(irVersion: "1.2", ir: #"{"irVersion":"1.1"}"#),
                expectedGameID: "game-123"
            )
        ) { error in
            XCTAssertEqual(error as? PlayerFailure, .irVersionMismatch)
        }
    }

    func testRejectsMalformedAssetReference() {
        let assets = #"[{"id":"","kind":"image","uri":"sprite.png","url":"/asset"}]"#
        XCTAssertThrowsError(try PlayerContract().validate(package(assets: assets), expectedGameID: "game-123"))
    }
}
