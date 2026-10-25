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
- This core validates the envelope shape and the format of SHA-256 fields; it does **not** calculate the IR's canonical SHA-256 or verify downloaded asset bytes. The runtime host must verify the exact canonicalization contract and asset hashes before execution.
- This package does not itself create a WKWebView, perform networking, or claim device-level lifecycle validation. Those remain integration tasks for the iOS app and Xcode simulator/device pipeline.
