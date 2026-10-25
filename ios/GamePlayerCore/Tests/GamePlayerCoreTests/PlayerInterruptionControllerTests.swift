import XCTest
@testable import GamePlayerCore

@available(iOS 16.0, macOS 13.0, *)
@MainActor
final class PlayerInterruptionControllerTests: XCTestCase {
    func testPausesAndResumesOnlyAfterEligibleInterruption() {
        let session = PlayerSessionModel()
        session.beginLoad(title: "Test")
        session.packageReady(gameID: "game-1", version: "v1")
        session.start()
        var pauses = 0
        var resumes = 0
        let controller = PlayerInterruptionController(
            session: session,
            pauseRuntime: { pauses += 1 },
            resumeRuntime: { resumes += 1 }
        )

        controller.interruptionBegan()
        XCTAssertEqual(session.state, .paused(gameID: "game-1", packageVersion: "v1", reason: .interruption))
        XCTAssertEqual(pauses, 1)
        controller.interruptionEnded(shouldResume: true)
        XCTAssertEqual(session.state, .playing(gameID: "game-1", packageVersion: "v1"))
        XCTAssertEqual(resumes, 1)
    }

    func testDoesNotResumeUserPausedOrNonResumableSession() {
        let session = PlayerSessionModel()
        session.beginLoad(title: "Test")
        session.packageReady(gameID: "game-1", version: "v1")
        session.start()
        session.pauseByUser()
        var resumes = 0
        let controller = PlayerInterruptionController(
            session: session,
            pauseRuntime: {},
            resumeRuntime: { resumes += 1 }
        )

        controller.interruptionBegan()
        controller.interruptionEnded(shouldResume: true)
        XCTAssertEqual(session.state, .paused(gameID: "game-1", packageVersion: "v1", reason: .user))
        XCTAssertEqual(resumes, 0)

        session.resume()
        controller.interruptionBegan()
        controller.interruptionEnded(shouldResume: false)
        XCTAssertEqual(session.state, .paused(gameID: "game-1", packageVersion: "v1", reason: .interruption))
        XCTAssertEqual(resumes, 0)
    }
}
