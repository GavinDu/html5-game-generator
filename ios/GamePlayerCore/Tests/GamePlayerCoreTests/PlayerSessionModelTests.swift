import XCTest
@testable import GamePlayerCore

@MainActor
final class PlayerSessionModelTests: XCTestCase {
    func testLoadProgressIsClampedAndClearedOnFailure() {
        let session = PlayerSessionModel()
        session.beginLoad(title: "Space cats")
        XCTAssertEqual(session.state, .loading)
        session.updateLoadProgress(1.5)
        XCTAssertEqual(session.loadProgress, 1)
        session.fail(.network("offline"))
        XCTAssertEqual(session.state, .failed(.network("offline")))
        XCTAssertNil(session.loadProgress)
    }

    func testBackgroundAndInterruptionRecovery() {
        let session = PlayerSessionModel()
        session.beginLoad(title: "Puzzle")
        session.packageReady(gameID: "g-1", version: "v4")
        session.start()
        session.pauseForBackground()
        XCTAssertEqual(session.state, .paused(gameID: "g-1", packageVersion: "v4", reason: .appBackgrounded))
        session.resume()
        XCTAssertEqual(session.state, .playing(gameID: "g-1", packageVersion: "v4"))
        session.pauseForInterruption()
        XCTAssertEqual(session.state, .paused(gameID: "g-1", packageVersion: "v4", reason: .interruption))
    }

    func testRetryStartsFreshLoadingStateAndResetClearsMetadata() {
        let session = PlayerSessionModel()
        session.beginLoad(title: "Runner")
        session.packageReady(gameID: "g-2", version: "v1")
        session.fail(.httpStatus(503))
        session.retry()
        XCTAssertEqual(session.state, .loading)
        XCTAssertEqual(session.loadProgress, 0)
        session.reset()
        XCTAssertEqual(session.state, .idle)
        XCTAssertEqual(session.gameTitle, "")
        XCTAssertNil(session.loadProgress)
    }
}
