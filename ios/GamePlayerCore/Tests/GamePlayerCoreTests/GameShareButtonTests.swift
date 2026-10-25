import XCTest
@testable import GamePlayerCore

@available(iOS 16.0, macOS 13.0, *)
final class GameShareButtonTests: XCTestCase {
    func testShareButtonUsesTrustedVersionedShareBuilder() throws {
        let builder = try GameShareLinkBuilder(baseURL: URL(string: "https://games.example.com")!)
        let lineage = try GameShareLineage(gameID: "game-123", version: "v7", parentGameID: "parent-9")
        let button = try GameShareButton(title: "Space Quest", lineage: lineage, builder: builder)

        let components = URLComponents(url: button.shareURL, resolvingAgainstBaseURL: false)
        XCTAssertEqual(components?.scheme, "https")
        XCTAssertEqual(components?.host, "games.example.com")
        XCTAssertEqual(components?.path, "/games/game-123")
        XCTAssertEqual(components?.queryItems?.first(where: { $0.name == "version" })?.value, "v7")
        XCTAssertEqual(components?.queryItems?.first(where: { $0.name == "parentGameId" })?.value, "parent-9")
    }
}
