import CryptoKit
import XCTest
@testable import GamePlayerCore

private struct StubPackageTransport: GamePackageTransport {
    let data: Data
    func fetchPackage(gameID: String, version: String?) async throws -> Data { data }
}

private struct StubAssetTransport: GameAssetTransport {
    let responses: [String: Data]
    func fetchAsset(_ asset: PackageAsset) async throws -> Data {
        guard let data = responses[asset.id] else { throw PlayerFailure.network("missing fixture asset") }
        return data
    }
}

private actor StubRuntimeHost: GameRuntimeHost {
    private(set) var starts = 0
    private(set) var lastAssets: [String: Data] = [:]
    private(set) var paused = false
    private(set) var stopped = false

    func start(package: GamePackageEnvelope, assets: [String: Data]) async throws {
        starts += 1
        lastAssets = assets
    }
    func pause() async { paused = true }
    func resume() async throws { paused = false }
    func stop() async { stopped = true }
}

final class PlayerLaunchCoordinatorTests: XCTestCase {
    private func fixturePackage(assetBytes: Data? = nil) -> (Data, [String: Data]) {
        let bytes = assetBytes ?? Data("sprite bytes".utf8)
        let hash = SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
        let json = """
        {
          "packageFormatVersion":"2.0",
          "minRuntimeVersion":"2026.10.23.2",
          "gameId":"game-123",
          "ir":{"schemaVersion":"1.0","nodes":[]},
          "assets":[{"id":"sprite","kind":"image","uri":"assets/sprite.png","url":"/v1/games/game-123/assets/sprite","mimeType":"image/png","sha256":"\(hash)"}],
          "irHash":"\(String(repeating: "a", count: 64))"
        }
        """
        return (Data(json.utf8), ["sprite": bytes])
    }

    func testLaunchVerifiesAssetBytesBeforeStartingRuntime() async throws {
        let (package, assets) = fixturePackage()
        let runtime = StubRuntimeHost()
        let coordinator = PlayerLaunchCoordinator(
            packageTransport: StubPackageTransport(data: package),
            assetTransport: StubAssetTransport(responses: assets),
            runtime: runtime
        )

        try await coordinator.launch(gameID: "game-123", version: "v7")

        let starts = await runtime.starts
        let loadedAssets = await runtime.lastAssets
        let stage = await coordinator.stage
        XCTAssertEqual(starts, 1)
        XCTAssertEqual(loadedAssets["sprite"], assets["sprite"])
        XCTAssertEqual(stage, .running)
    }

    func testHashMismatchFailsClosedAndDoesNotStartRuntime() async {
        let (package, _) = fixturePackage()
        let runtime = StubRuntimeHost()
        let coordinator = PlayerLaunchCoordinator(
            packageTransport: StubPackageTransport(data: package),
            assetTransport: StubAssetTransport(responses: ["sprite": Data("tampered".utf8)]),
            runtime: runtime
        )

        do {
            try await coordinator.launch(gameID: "game-123")
            XCTFail("tampered asset must not launch")
        } catch let error as PlayerLaunchError {
            XCTAssertEqual(error, .assetHashMismatch("sprite"))
        } catch {
            XCTFail("unexpected error: \(error)")
        }

        let starts = await runtime.starts
        let stage = await coordinator.stage
        XCTAssertEqual(starts, 0)
        XCTAssertEqual(stage, .failed(.malformedPackage))
    }

    func testPrivateAssetWithoutHashFailsClosed() async {
        let bytes = Data("sprite bytes".utf8)
        let json = """
        {
          "packageFormatVersion":"2.0",
          "minRuntimeVersion":"2026.10.23.2",
          "gameId":"game-123",
          "ir":{"schemaVersion":"1.0"},
          "assets":[{"id":"sprite","kind":"image","uri":"sprite.png","url":"/asset"}],
          "irHash":"\(String(repeating: "a", count: 64))"
        }
        """
        let runtime = StubRuntimeHost()
        let coordinator = PlayerLaunchCoordinator(
            packageTransport: StubPackageTransport(data: Data(json.utf8)),
            assetTransport: StubAssetTransport(responses: ["sprite": bytes]),
            runtime: runtime
        )
        do {
            try await coordinator.launch(gameID: "game-123")
            XCTFail("asset without hash must not launch")
        } catch let error as PlayerLaunchError {
            XCTAssertEqual(error, .missingAssetHash("sprite"))
        } catch {
            XCTFail("unexpected error: \(error)")
        }
        let starts = await runtime.starts
        XCTAssertEqual(starts, 0)
    }
}
