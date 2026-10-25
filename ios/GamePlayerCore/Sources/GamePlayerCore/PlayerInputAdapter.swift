import Foundation

/// Platform-neutral input events. UIKit, GameController, and WKWebView adapters
/// should translate platform events into this type instead of implementing
/// device-specific game semantics in each player surface.
public enum PlayerInputEvent: Equatable, Sendable {
    case direction(x: Double, y: Double)
    case button(name: PlayerInputButton, pressed: Bool)
    case pointer(PlayerPointerInput)
}

public enum PlayerInputButton: String, CaseIterable, Equatable, Sendable {
    case confirm
    case cancel
    case primary
    case secondary
    case pause
    case restart
}

/// Converts keyboard/gamepad directional values to stable normalized axes.
/// Positive x points right; positive y points down, matching canvas coordinates.
public struct PlayerInputAdapter: Equatable, Sendable {
    public private(set) var x: Double = 0
    public private(set) var y: Double = 0
    public private(set) var pressedButtons: Set<PlayerInputButton> = []

    public init() {}

    public mutating func apply(_ event: PlayerInputEvent) {
        switch event {
        case let .direction(x, y):
            self.x = Self.normalizeAxis(x)
            self.y = Self.normalizeAxis(y)
        case let .button(name, pressed):
            if pressed {
                pressedButtons.insert(name)
            } else {
                pressedButtons.remove(name)
            }
        case .pointer:
            // Pointer coordinates are delivered as discrete events to the
            // renderer; they must not silently alter directional/button state.
            break
        }
    }

    public mutating func reset() {
        x = 0
        y = 0
        pressedButtons.removeAll(keepingCapacity: true)
    }

    public static func direction(for key: String) -> (x: Double, y: Double)? {
        switch key.lowercased() {
        case "arrowleft", "a", "a11yleft": return (-1, 0)
        case "arrowright", "d", "a11yright": return (1, 0)
        case "arrowup", "w", "a11yup": return (0, -1)
        case "arrowdown", "s", "a11ydown": return (0, 1)
        default: return nil
        }
    }

    private static func normalizeAxis(_ value: Double) -> Double {
        guard value.isFinite else { return 0 }
        return min(max(value, -1), 1)
    }
}
