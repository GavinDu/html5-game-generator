import Foundation

/// Renderer-neutral input vocabulary. Platform adapters (UIKit, pointer events,
/// keyboard, and game controllers) translate into this type so gameplay logic
/// does not branch on the originating device.
public enum PlayerInputAction: String, Equatable, Sendable {
    case moveUp
    case moveDown
    case moveLeft
    case moveRight
    case confirm
    case cancel
    case pause
    case primary
    case secondary
}

public enum PlayerInputPhase: String, Equatable, Sendable {
    case began
    case changed
    case ended
    case cancelled
}

public struct PlayerInputEvent: Equatable, Sendable {
    public let action: PlayerInputAction
    public let phase: PlayerInputPhase
    public let source: Source

    public enum Source: String, Equatable, Sendable {
        case touch
        case pointer
        case keyboard
        case gameController
    }

    public init(action: PlayerInputAction, phase: PlayerInputPhase, source: Source) {
        self.action = action
        self.phase = phase
        self.source = source
    }
}

/// Stable key mapping for host adapters. The adapter should emit the ended
/// phase when a key is released and suppress repeats for edge-triggered actions.
public enum PlayerKeyboardMapping {
    public static func action(for key: String) -> PlayerInputAction? {
        switch key.lowercased() {
        case "arrowup", "w", "k": return .moveUp
        case "arrowdown", "s", "j": return .moveDown
        case "arrowleft", "a", "h": return .moveLeft
        case "arrowright", "d", "l": return .moveRight
        case " ", "space", "enter", "return": return .confirm
        case "escape", "esc", "backspace": return .cancel
        case "p", "pause": return .pause
        case "z", "x": return .primary
        case "c": return .secondary
        default: return nil
        }
    }
}

/// A small common subset of controller controls. Platform-specific gamepad
/// adapters can map native button identifiers to these semantic actions.
public enum PlayerControllerControl: String, Equatable, Sendable {
    case dpadUp
    case dpadDown
    case dpadLeft
    case dpadRight
    case faceSouth
    case faceEast
    case menu
    case leftShoulder
    case rightShoulder

    public var action: PlayerInputAction {
        switch self {
        case .dpadUp: return .moveUp
        case .dpadDown: return .moveDown
        case .dpadLeft: return .moveLeft
        case .dpadRight: return .moveRight
        case .faceSouth: return .confirm
        case .faceEast: return .cancel
        case .menu: return .pause
        case .leftShoulder: return .secondary
        case .rightShoulder: return .primary
        }
    }
}
