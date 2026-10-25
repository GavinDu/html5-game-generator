import XCTest
@testable import GamePlayerCore

final class PlayerViewportPolicyTests: XCTestCase {
    func testAspectFitUsesLetterboxingInsteadOfStretching() {
        let rect = PlayerViewportPolicy.aspectFit(
            container: PlayerSize(width: 400, height: 800),
            gameAspectRatio: 16.0 / 9.0
        )
        XCTAssertEqual(rect?.width ?? 0, 400, accuracy: 0.001)
        XCTAssertEqual(rect?.height ?? 0, 225, accuracy: 0.001)
        XCTAssertEqual(rect?.y ?? 0, 287.5, accuracy: 0.001)
    }

    func testSafeAreaInsetsAreExcludedFromAvailableBounds() {
        let rect = PlayerViewportPolicy.aspectFit(
            container: PlayerSize(width: 400, height: 800),
            safeArea: PlayerInsets(top: 50, leading: 10, bottom: 30, trailing: 10),
            gameAspectRatio: 1
        )
        XCTAssertEqual(rect?.x ?? 0, 10, accuracy: 0.001)
        XCTAssertEqual(rect?.y ?? 0, 365, accuracy: 0.001)
        XCTAssertEqual(rect?.width ?? 0, 380, accuracy: 0.001)
        XCTAssertEqual(rect?.height ?? 0, 380, accuracy: 0.001)
    }

    func testRejectsInvalidAspectRatioAndEmptyViewport() {
        XCTAssertNil(PlayerViewportPolicy.aspectFit(container: PlayerSize(width: 400, height: 800), gameAspectRatio: 0))
        XCTAssertNil(PlayerViewportPolicy.aspectFit(container: PlayerSize(width: 0, height: 800), gameAspectRatio: 1))
        XCTAssertNil(PlayerViewportPolicy.aspectFit(container: PlayerSize(width: 400, height: 800), gameAspectRatio: .infinity))
    }

    func testOrientationChangesAfterRotation() {
        XCTAssertEqual(PlayerViewportPolicy.orientation(for: PlayerSize(width: 390, height: 844)), .portrait)
        XCTAssertEqual(PlayerViewportPolicy.orientation(for: PlayerSize(width: 844, height: 390)), .landscape)
        XCTAssertEqual(PlayerViewportPolicy.orientation(for: PlayerSize(width: 400, height: 400)), .square)
    }

    func testPointerCoordinatesAreClampedAndNonFiniteValuesNormalized() {
        XCTAssertEqual(PlayerPointerInput(x: 1.4, y: -0.5, phase: .began), PlayerPointerInput(x: 1, y: 0, phase: .began))
        XCTAssertEqual(PlayerPointerInput(x: .nan, y: .infinity, phase: .moved), PlayerPointerInput(x: 0, y: 0, phase: .moved))
    }
}
