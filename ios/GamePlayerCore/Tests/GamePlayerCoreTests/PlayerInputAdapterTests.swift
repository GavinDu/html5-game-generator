import XCTest
@testable import GamePlayerCore

final class PlayerInputAdapterTests: XCTestCase {
    func testKeyboardAliasesMapToCanvasDirections() {
        XCTAssertEqual(PlayerInputAdapter.direction(for: "ArrowLeft")?.x, -1)
        XCTAssertEqual(PlayerInputAdapter.direction(for: "a")?.x, -1)
        XCTAssertEqual(PlayerInputAdapter.direction(for: "ARROWDOWN")?.y, 1)
        XCTAssertEqual(PlayerInputAdapter.direction(for: "w")?.y, -1)
        XCTAssertNil(PlayerInputAdapter.direction(for: "Escape"))
    }

    func testDirectionalAxesAreClampedAndNonFiniteValuesNormalized() {
        var input = PlayerInputAdapter()
        input.apply(.direction(x: 2, y: -.infinity))
        XCTAssertEqual(input.x, 1)
        XCTAssertEqual(input.y, 0)
        input.apply(.direction(x: .nan, y: -0.25))
        XCTAssertEqual(input.x, 0)
        XCTAssertEqual(input.y, -0.25)
    }

    func testButtonsRemainPressedUntilReleaseAndResetClearsState() {
        var input = PlayerInputAdapter()
        input.apply(.button(name: .confirm, pressed: true))
        input.apply(.button(name: .pause, pressed: true))
        input.apply(.button(name: .confirm, pressed: false))
        XCTAssertEqual(input.pressedButtons, [.pause])
        input.reset()
        XCTAssertEqual(input.pressedButtons, [])
        XCTAssertEqual(input.x, 0)
        XCTAssertEqual(input.y, 0)
    }

    func testPointerEventsDoNotMutateKeyboardOrGamepadState() {
        var input = PlayerInputAdapter()
        input.apply(.direction(x: 0.5, y: -0.5))
        input.apply(.pointer(PlayerPointerInput(x: 0.2, y: 0.8, phase: .began)))
        XCTAssertEqual(input.x, 0.5)
        XCTAssertEqual(input.y, -0.5)
    }
}
