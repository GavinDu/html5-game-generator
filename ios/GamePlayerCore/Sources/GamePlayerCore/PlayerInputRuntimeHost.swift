import Foundation

/// Optional runtime capability for forwarding platform-neutral player input.
/// Hosts that don't implement this capability explicitly report input as
/// unavailable through PlayerLaunchCoordinator instead of silently dropping it.
public protocol PlayerInputRuntimeHost: GameRuntimeHost {
    func sendInput(_ event: PlayerInputEvent) async
}
