import XCTest
@testable import GamePlayerCore

final class URLSessionGameTransportTests: XCTestCase {
    func testRejectsNonTLSProductionBaseURL() {
        XCTAssertThrowsError(try URLSessionGameTransport(baseURL: URL(string: "http://api.example.com")!)) { error in
            XCTAssertEqual(error as? GameTransportError, .invalidBaseURL)
        }
    }

    func testAllowsLocalDevelopmentHTTPBaseURL() throws {
        XCTAssertNoThrow(try URLSessionGameTransport(baseURL: URL(string: "http://127.0.0.1:8080")!))
    }

    func testRejectsAssetURLOnForeignOriginBeforeNetworkRequest() async throws {
        let transport = try URLSessionGameTransport(baseURL: URL(string: "https://api.example.com")!)
        let asset = PackageAsset(
            id: "sprite",
            kind: "image",
            uri: "sprite.png",
            url: "https://attacker.example/collect",
            sha256: String(repeating: "a", count: 64)
        )
        do {
            _ = try await transport.fetchAsset(asset)
            XCTFail("foreign origin should be rejected before networking")
        } catch let error as GameTransportError {
            XCTAssertEqual(error, .rejectedURL)
        }
    }

    func testRejectsCredentialsAndForeignPort() async throws {
        let transport = try URLSessionGameTransport(baseURL: URL(string: "https://api.example.com")!)
        for url in [
            "https://user:password@api.example.com/private",
            "https://api.example.com:9443/private",
            "file:///etc/passwd"
        ] {
            let asset = PackageAsset(id: "a", kind: "image", uri: "a", url: url)
            do {
                _ = try await transport.fetchAsset(asset)
                XCTFail("expected rejected URL: \(url)")
            } catch let error as GameTransportError {
                XCTAssertEqual(error, .rejectedURL)
            }
        }
    }
}
