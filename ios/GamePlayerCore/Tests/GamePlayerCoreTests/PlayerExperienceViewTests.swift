import XCTest
import SwiftUI
@testable import GamePlayerCore

@available(iOS 16.0, macOS 13.0, *)
final class PlayerExperienceViewTests: XCTestCase {
    func testAspectRatioIsClampedToSupportedRange() {
        let tooNarrow = PlayerExperienceView(
            state: .idle,
            title: "Test",
            gameAspectRatio: 0,
            renderer: { EmptyView() },
            onPlay: {}, onPause: {}, onResume: {}, onRetry: {}, onExit: {}, onRestart: {}
        )
        XCTAssertEqual(tooNarrow.gameAspectRatio, 0.25)

        let tooWide = PlayerExperienceView(
            state: .idle,
            title: "Test",
            gameAspectRatio: 99,
            renderer: { EmptyView() },
            onPlay: {}, onPause: {}, onResume: {}, onRetry: {}, onExit: {}, onRestart: {}
        )
        XCTAssertEqual(tooWide.gameAspectRatio, 4.0)
    }

    func testNonFiniteAspectRatioUsesLandscapeDefault() {
        let view = PlayerExperienceView(
            state: .idle,
            title: "Test",
            gameAspectRatio: .infinity,
            renderer: { EmptyView() },
            onPlay: {}, onPause: {}, onResume: {}, onRetry: {}, onExit: {}, onRestart: {}
        )
        XCTAssertEqual(view.gameAspectRatio, 16.0 / 9.0, accuracy: 0.0001)
    }
}
