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
        irHash: String = String(repeating: "a", count: 64),
        assets: String = "[]"
    ) -> Data {
        Data("""
        {
          "packageFormatVersion":"\(format)",
          "minRuntimeVersion":"\(runtime)",
          "irVersion":"\(irVersion)",
          "gameId":"\(gameID)",
          "ir":\(ir),
          "assets":\(assets),
          "irHash":"\(irHash)"
        }
        """.utf8)
    }

    func testAcceptsSupportedPackageEnvelope() throws {
        let result = try PlayerContract().validate(package(), expectedGameID: "game-123")
        XCTAssertEqual(result.gameId, "game-123")
        XCTAssertEqual(result.irVersion, "1.0")
        XCTAssertEqual(result.assets.count, 0)
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
