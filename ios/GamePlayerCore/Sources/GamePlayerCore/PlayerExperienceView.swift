import SwiftUI

/// A reusable, accessible player shell. The runtime renderer is supplied by the
/// host app; this view owns the surrounding loading/error/pause/completion UX.
@available(iOS 16.0, macOS 13.0, *)
public struct PlayerExperienceView<Renderer: View>: View {
    public let state: PlayerState
    public let title: String
    public let progress: Double?
    public let renderer: Renderer
    public let onPlay: () -> Void
    public let onPause: () -> Void
    public let onResume: () -> Void
    public let onRetry: () -> Void
    public let onExit: () -> Void
    public let onRestart: () -> Void

    public init(
        state: PlayerState,
        title: String,
        progress: Double? = nil,
        @ViewBuilder renderer: () -> Renderer,
        onPlay: @escaping () -> Void,
        onPause: @escaping () -> Void,
        onResume: @escaping () -> Void,
        onRetry: @escaping () -> Void,
        onExit: @escaping () -> Void,
        onRestart: @escaping () -> Void
    ) {
        self.state = state
        self.title = title
        self.progress = progress
        self.renderer = renderer()
        self.onPlay = onPlay
        self.onPause = onPause
        self.onResume = onResume
        self.onRetry = onRetry
        self.onExit = onExit
        self.onRestart = onRestart
    }

    public var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 12) {
                Button(action: onExit) {
                    Label("Exit game", systemImage: "xmark")
                        .labelStyle(.iconOnly)
                        .frame(minWidth: 44, minHeight: 44)
                }
                .accessibilityLabel("Exit game")

                Text(title)
                    .font(.headline)
                    .lineLimit(2)
                    .frame(maxWidth: .infinity, alignment: .leading)

                stateIndicator
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)

            ZStack {
                Color.black
                renderer
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .accessibilityHidden(isOverlayBlocking)

                if isOverlayBlocking {
                    overlay
                        .padding(24)
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                        .background(.ultraThinMaterial)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .padding(.horizontal, 12)

            HStack(spacing: 12) {
                switch state {
                case .ready:
                    Button(action: onPlay) { Label("Play", systemImage: "play.fill") }
                        .buttonStyle(.borderedProminent)
                case .playing:
                    Button(action: onPause) { Label("Pause", systemImage: "pause.fill") }
                        .buttonStyle(.borderedProminent)
                case .paused:
                    Button(action: onResume) { Label("Resume", systemImage: "play.fill") }
                        .buttonStyle(.borderedProminent)
                case .completed:
                    Button(action: onRestart) { Label("Play again", systemImage: "arrow.clockwise") }
                        .buttonStyle(.borderedProminent)
                case .failed:
                    Button(action: onRetry) { Label("Retry", systemImage: "arrow.clockwise") }
                        .buttonStyle(.borderedProminent)
                    Button(action: onExit) { Label("Exit", systemImage: "xmark") }
                        .buttonStyle(.bordered)
                case .idle, .loading:
                    EmptyView()
                }

                Spacer(minLength: 8)

                if let progress, state == .loading {
                    ProgressView(value: min(max(progress, 0), 1))
                        .frame(maxWidth: 120)
                        .accessibilityLabel("Game loading progress")
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
        }
        .background(Color.primary.opacity(0.04))
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder
    private var stateIndicator: some View {
        switch state {
        case .loading:
            ProgressView().accessibilityLabel("Loading game")
        case .paused:
            Label("Paused", systemImage: "pause.circle").font(.caption)
        case .failed:
            Label("Needs attention", systemImage: "exclamationmark.triangle")
                .font(.caption)
                .foregroundStyle(.orange)
        case .completed:
            Label("Completed", systemImage: "checkmark.circle").font(.caption)
        default:
            EmptyView()
        }
    }

    private var isOverlayBlocking: Bool {
        switch state {
        case .idle, .loading, .failed, .completed:
            return true
        default:
            return false
        }
    }

    @ViewBuilder
    private var overlay: some View {
        VStack(spacing: 12) {
            switch state {
            case .idle:
                Image(systemName: "gamecontroller").font(.largeTitle)
                Text("Choose a game to start").font(.headline)
            case .loading:
                ProgressView()
                Text("Preparing your game").font(.headline)
                Text("This can take a moment on a slow connection.")
                    .font(.subheadline)
                    .multilineTextAlignment(.center)
            case .failed(let failure):
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(.largeTitle)
                    .foregroundStyle(.orange)
                Text("Couldn’t load this game").font(.headline)
                Text(failureMessage(failure))
                    .font(.subheadline)
                    .multilineTextAlignment(.center)
                Button("Try again", action: onRetry).buttonStyle(.borderedProminent)
                Button("Exit game", action: onExit).buttonStyle(.bordered)
            case .completed:
                Image(systemName: "checkmark.circle.fill")
                    .font(.largeTitle)
                    .foregroundStyle(.green)
                Text("You finished!").font(.headline)
                Button("Play again", action: onRestart).buttonStyle(.borderedProminent)
            case .paused(_, _, let reason):
                Image(systemName: "pause.circle.fill").font(.largeTitle)
                Text("Game paused").font(.headline)
                Text(reason == .appBackgrounded ? "Tap Resume when you’re ready to continue." : "Resume when you’re ready.")
                    .font(.subheadline)
                    .multilineTextAlignment(.center)
            case .ready, .playing:
                EmptyView()
            }
        }
        .frame(maxWidth: 360)
        .padding(24)
        .accessibilityElement(children: .contain)
    }

    private func failureMessage(_ failure: PlayerFailure) -> String {
        switch failure {
        case .invalidURL:
            return "The game link is invalid. Return to the feed and open the game again."
        case .network:
            return "Check your connection, then try loading the game again."
        case .httpStatus(let code):
            return "The server couldn’t load this game (HTTP \(code)). You can retry or exit."
        case .unsupportedPackageFormat, .unsupportedRuntime:
            return "This game needs a newer player version. Update the app and try again."
        case .packageGameIDMismatch, .missingIR, .invalidIRHash, .assetHashMissing, .assetHashMismatch, .assetTooLarge, .assetOriginMismatch, .malformedPackage:
            return "The game package could not be verified. It wasn’t started. Try again later."
        case .cancelled:
            return "Loading was interrupted. Try again when you’re ready."
        }
    }
}
