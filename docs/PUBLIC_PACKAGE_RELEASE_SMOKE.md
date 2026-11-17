# Public package release smoke (Epic #202 / atomic #491)

The canonical backend source sends a Game Package **2.0** JSON envelope, but the November 15, 2026 live native CI reported three owner-linked public games whose separately fetched packages lacked `packageFormatVersion`. This is a deployment/serializer contract mismatch, **not** a reason to accept arbitrary older formats in the iOS player.

After an authorized maintainer deploys and verifies the desired backend release SHA, verify a **known, real, public and published** game from that exact origin:

```sh
go run ./cmd/verify-live-package \
  -base-url https://your-deployed-backend.example \
  -game-id REAL_PUBLIC_PUBLISHED_GAME_ID
```

This CLI is opt-in; it never initiates deployment. It uses a 10-second HTTPS GET with redirects denied, reads at most 8 MiB, validates the canonical Go `api.GamePackage` contract (package format/minimum runtime/IR version/game ID), strict Go Game IR validation, full SHA-256 of typed Go-serialized IR, short legacy prefix and asset digests. It deliberately does **not** log the game ID, package contents, URL, creator/owner name, credentials, or full SHA. Success prints only format, IR version and asset count. Failure prints a bounded reason code such as `missing_package_format_version`, `package_http_status_404` or `package_full_ir_sha256_mismatch`.

This check is not a replacement for the independent native [real owner-linked published game preflight](https://github.com/GavinDu/Hephaestus/issues/345). It validates one operator-supplied package, not feed visibility, moderator approval, a valid creator profile, CDN asset retrieval, current player WebKit lifecycle, source-independent score attestation, GPU compositor-present or device thermal performance. Do not treat a green offline unit test as proof that the stale public deployment has been updated.

## Release gate and rollback

1. Identify the authorized backend release SHA and service origin, and compare deployed package format to current Go source `internal/api/package.go`; inspect only sanitized field presence, not private Game IR payload or credentials.
2. Publish/deploy that exact canonical server release using the operator's authorized deployment workflow. No Replit AI model is needed by this CLI. If the deployed server is stale or has a proxy rewriting JSON, repair that layer and any affected public cache rather than weakening the client parser.
3. Run the CLI on a real published game and capture the safe result alongside the release SHA. If red, **do not** mark native iPhone/iPad CI green or bypass its mandatory live tests.
4. Independently run the actual native feed/package fixture gate and all iPhone/iPad UI smoke. Roll back a misconfigured backend release using normal deployment controls, preserving historical game/ranking data.

All offline negative tests use local `httptest.NewTLSServer` and the real shared Go/Pixi startup-score Game IR, never fabricated public production data. The parent [Hephaestus #368](https://github.com/GavinDu/Hephaestus/issues/368), creator fixture #288, native CI #345 and Epic #202 remain OPEN until their own acceptance evidence exists.
