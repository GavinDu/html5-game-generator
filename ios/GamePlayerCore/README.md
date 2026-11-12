# iOS Game Player Core

This Swift Package contains platform-independent contracts and lifecycle state for the iOS game player. It is deliberately separate from the SwiftUI/WKWebView host so the package contract and interruption behavior can be tested on GitHub Actions without a simulator.

## Validate locally

From this directory:

```sh
swift test
```

## Integration notes

- Use `PlayerContract.validate(_:expectedGameID:)` after fetching `GET /v1/games/{id}/package` and before loading the runtime.
- Keep the server-provided package envelope as the source of truth. A mismatch, unsupported package/runtime version, missing IR, or malformed asset reference should render an actionable error state with Retry and Exit actions; do not execute an unknown package.
- Map app backgrounding and audio interruptions to `pause(_:) `; resume only after the app is active and the runtime has acknowledged it is ready.
- Map network, decoding, and runtime failures to `fail(_:)`; Retry starts a fresh load and must not continue using a stale game session.
- This core validates the package envelope and calculates the SHA-256 of the exact embedded Game IR JSON bytes as emitted by Go's `json.Marshal`. It checks `irSha256` (full 64-hex digest) and that `irHash` (the historical 12-hex run/leaderboard key) is its prefix. It deliberately does **not** hash a re-encoded Swift dictionary, because its key order can differ from Go's canonical struct encoding. A prettified or reserialized package may fail integrity checks even when semantically equivalent; the backend delivers compact canonical bytes.
- `PlayerLaunchCoordinator` separately verifies each downloaded asset's SHA-256 before starting a runtime host. The production Hephaestus WKWebView is currently an independent host; passing these Swift package tests does not prove that host is integrated.
- The `iOS Player Core` GitHub workflow generates a real Game Package through the Go HTTP handler and passes it to Swift tests via `GAME_PACKAGE_FIXTURE_PATH`; local `swift test` also exercises synthetic exact-byte fixtures.
- This package does not itself create a WKWebView, perform networking, or claim device-level lifecycle validation. Those remain integration tasks for the iOS app and Xcode simulator/device pipeline.
