import XCTest
@testable import GamePlayerCore

final class PlayerInputMappingTests: XCTestCase {
    func testDirectionalKeyboardAliasesMapConsistently() {
        for key in ["ArrowUp", "W", "k"] { XCTAssertEqual(PlayerKeyboardMapping.action(for: key), .moveUp) }
        for key in ["ArrowDown", "S", "j"] { XCTAssertEqual(PlayerKeyboardMapping.action(for: key), .moveDown) }
        for key in ["ArrowLeft", "A", "h"] { XCTAssertEqual(PlayerKeyboardMapping.action(for: key), .moveLeft) }
        for key in ["ArrowRight", "D", "l"] { XCTAssertEqual(PlayerKeyboardMapping.action(for: key), .moveRight) }
    }

    func testActionAndMenuKeys() {
        XCTAssertEqual(PlayerKeyboardMapping.action(for: "Enter"), .confirm)
        XCTAssertEqual(PlayerKeyboardMapping.action(for: "Escape"), .cancel)
        XCTAssertEqual(PlayerKeyboardMapping.action(for: "P"), .pause)
        XCTAssertEqual(PlayerKeyboardMapping.action(for: "C"), .secondary)
        XCTAssertNil(PlayerKeyboardMapping.action(for: "UnmappedKey"))
    }

    func testControllerControlsMapToSameSemanticActions() {
        XCTAssertEqual(PlayerControllerControl.dpadUp.action, .moveUp)
        XCTAssertEqual(PlayerControllerControl.faceSouth.action, .confirm)
        XCTAssertEqual(PlayerControllerControl.faceEast.action, .cancel)
        XCTAssertEqual(PlayerControllerControl.menu.action, .pause)
    }

    func testInputEventPreservesPhaseAndSource() {
        XCTAssertEqual(
            PlayerInputEvent(action: .primary, phase: .ended, source: .gameController),
            PlayerInputEvent(action: .primary, phase: .ended, source: .gameController)
        )
    }
}
