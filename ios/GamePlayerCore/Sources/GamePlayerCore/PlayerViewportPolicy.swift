import Foundation

public enum PlayerOrientation: Equatable, Sendable {
    case portrait
    case landscape
    case square
}

public struct PlayerInsets: Equatable, Sendable {
    public let top: Double
    public let leading: Double
    public let bottom: Double
    public let trailing: Double

    public init(top: Double = 0, leading: Double = 0, bottom: Double = 0, trailing: Double = 0) {
        self.top = max(0, top.isFinite ? top : 0)
        self.leading = max(0, leading.isFinite ? leading : 0)
        self.bottom = max(0, bottom.isFinite ? bottom : 0)
        self.trailing = max(0, trailing.isFinite ? trailing : 0)
    }
}

public struct PlayerSize: Equatable, Sendable {
    public let width: Double
    public let height: Double

    public init(width: Double, height: Double) {
        self.width = max(0, width.isFinite ? width : 0)
        self.height = max(0, height.isFinite ? height : 0)
    }
}

public struct PlayerRect: Equatable, Sendable {
    public let x: Double
    public let y: Double
    public let width: Double
    public let height: Double
}

/// Deterministic aspect-fit policy shared by web/iOS player hosts. Coordinates
/// are relative to the container after safe-area insets are applied. The runtime
/// must use this rectangle rather than stretching the game to fill the screen.
public enum PlayerViewportPolicy {
    public static func orientation(for size: PlayerSize) -> PlayerOrientation {
        if size.width > size.height { return .landscape }
        if size.height > size.width { return .portrait }
        return .square
    }

    public static func aspectFit(
        container: PlayerSize,
        safeArea: PlayerInsets = PlayerInsets(),
        gameAspectRatio: Double
    ) -> PlayerRect? {
        guard gameAspectRatio.isFinite, gameAspectRatio > 0 else { return nil }

        let availableWidth = max(0, container.width - safeArea.leading - safeArea.trailing)
        let availableHeight = max(0, container.height - safeArea.top - safeArea.bottom)
        guard availableWidth > 0, availableHeight > 0 else { return nil }

        let containerRatio = availableWidth / availableHeight
        let width: Double
        let height: Double
        if containerRatio > gameAspectRatio {
            height = availableHeight
            width = height * gameAspectRatio
        } else {
            width = availableWidth
            height = width / gameAspectRatio
        }

        return PlayerRect(
            x: safeArea.leading + (availableWidth - width) / 2,
            y: safeArea.top + (availableHeight - height) / 2,
            width: width,
            height: height
        )
    }
}

/// Normalized pointer data keeps touch/pointer coordinates independent of
/// device pixels and orientation. Consumers should clamp again at the renderer
/// boundary and must not infer privileged actions from untrusted game data.
public struct PlayerPointerInput: Equatable, Sendable {
    public let x: Double
    public let y: Double
    public let phase: Phase

    public enum Phase: String, Equatable, Sendable {
        case began
        case moved
        case ended
        case cancelled
    }

    public init(x: Double, y: Double, phase: Phase) {
        self.x = min(max(x.isFinite ? x : 0, 0), 1)
        self.y = min(max(y.isFinite ? y : 0, 0), 1)
        self.phase = phase
    }
}
