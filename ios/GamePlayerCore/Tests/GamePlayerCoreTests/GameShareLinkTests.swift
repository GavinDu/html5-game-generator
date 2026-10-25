import XCTest
@testable import GamePlayerCore

final class GameShareLinkTests: XCTestCase {
    func testRoundTripsVersionAndRemixParent() throws {
        let builder = try GameShareLinkBuilder(baseURL: URL(string: "https://games.example.test/app")!)
        let source = try GameShareLineage(gameID: "funny-cat", version: "v3", parentGameID: "original-cat")
        let url = try builder.makeLink(for: source)
        XCTAssertEqual(url.absoluteString, "https://games.example.test/app/games/funny-cat?version=v3&parentGameId=original-cat")
        XCTAssertEqual(try builder.parse(url), source)
    }

    func testRoundTripsEncodedIdentifiers() throws {
        let builder = try GameShareLinkBuilder(baseURL: URL(string: "https://games.example.test")!)
        let source = try GameShareLineage(gameID: "game with spaces", version: "v 2")
        let url = try builder.makeLink(for: source)
        XCTAssertEqual(try builder.parse(url), source)
    }

    func testRejectsNonHTTPSBaseAndForeignOrigin() throws {
        XCTAssertThrowsError(try GameShareLinkBuilder(baseURL: URL(string: "http://games.example.test")!))
        let builder = try GameShareLinkBuilder(baseURL: URL(string: "https://games.example.test")!)
        XCTAssertThrowsError(try builder.parse(URL(string: "https://evil.example.test/games/g?version=v1")!))
    }

    func testRejectsMissingVersionAndInvalidIdentifiers() throws {
        let builder = try GameShareLinkBuilder(baseURL: URL(string: "https://games.example.test")!)
        XCTAssertThrowsError(try builder.parse(URL(string: "https://games.example.test/games/g")!))
        XCTAssertThrowsError(try GameShareLineage(gameID: " ", version: "v1"))
        XCTAssertThrowsError(try GameShareLineage(gameID: "g", version: "v1", parentGameID: "bad\nparent"))
    }
}
