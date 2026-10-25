import XCTest
@testable import GamePlayerCore

final class GameDeepLinkRouterTests: XCTestCase {
    private let router = GameDeepLinkRouter(allowedHosts: ["games.example.com"])

    func testUniversalLinkPreservesVersionAndRemixLineage() throws {
        let route = try router.route(URL(string: "https://games.example.com/games/game-123?version=v7&parentGameId=parent-9")!)
        XCTAssertEqual(route, GameDeepLink(gameID: "game-123", version: "v7", remixParentID: "parent-9"))
    }

    func testCustomSchemeRoutesGameID() throws {
        let route = try router.route(URL(string: "gameplayer://games/game_1?version=2.0")!)
        XCTAssertEqual(route, GameDeepLink(gameID: "game_1", version: "2.0", remixParentID: nil))
    }

    func testShareURLRoundTripsVersionAndLineage() throws {
        let route = GameDeepLink(gameID: "game-123", version: "v7", remixParentID: "parent-9")
        let url = try router.shareURL(for: route, canonicalHost: "games.example.com")
        XCTAssertEqual(try router.route(url), route)
    }

    func testRejectsUntrustedHostAndScheme() {
        XCTAssertThrowsError(try router.route(URL(string: "https://evil.example/games/game-123")!))
        XCTAssertThrowsError(try router.route(URL(string: "javascript://games/game-123")!))
        XCTAssertThrowsError(try router.route(URL(string: "http://games.example.com/games/game-123")!))
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com:444/games/game-123")!))
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com/games/game-123#fragment")!))
    }

    func testRejectsAmbiguousVersionAndRemixQueryParameters() {
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com/games/game-123?version=v1&version=v2")!)) { error in
            XCTAssertEqual(error as? GameDeepLinkError, .duplicateQueryParameter)
        }
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com/games/game-123?parentGameId=p1&remixParent=p2")!)) { error in
            XCTAssertEqual(error as? GameDeepLinkError, .duplicateQueryParameter)
        }
    }

    func testRejectsPathTraversalAndInvalidIdentifiers() {
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com/games/../private")!))
        XCTAssertThrowsError(try router.route(URL(string: "https://games.example.com/games/game-123?version=../../secret")!))
    }
}
