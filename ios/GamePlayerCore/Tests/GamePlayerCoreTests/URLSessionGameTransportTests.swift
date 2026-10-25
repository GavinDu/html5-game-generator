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


private final class FixedResponseURLProtocol: URLProtocol {
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let bytes = Data("payload".utf8)
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Length": String(bytes.count)]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: bytes)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

extension URLSessionGameTransportTests {
    func testRejectsOversizedResponseWhileStreaming() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [FixedResponseURLProtocol.self]
        let session = URLSession(configuration: configuration)
        let transport = try URLSessionGameTransport(
            baseURL: URL(string: "https://api.example.com")!,
            session: session,
            packageByteLimit: 3
        )
        do {
            _ = try await transport.fetchPackage(gameID: "game-123", version: nil)
            XCTFail("response larger than the configured byte limit must be rejected")
        } catch let error as GameTransportError {
            XCTAssertEqual(error, .responseTooLarge(limit: 3))
        }
        session.invalidateAndCancel()
    }

    func testAcceptsResponseExactlyAtByteLimit() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [FixedResponseURLProtocol.self]
        let session = URLSession(configuration: configuration)
        let transport = try URLSessionGameTransport(
            baseURL: URL(string: "https://api.example.com")!,
            session: session,
            packageByteLimit: 7
        )
        let data = try await transport.fetchPackage(gameID: "game-123", version: nil)
        XCTAssertEqual(data, Data("payload".utf8))
        session.invalidateAndCancel()
    }
}


extension URLSessionGameTransportTests {
    func testRedirectPolicyRejectsCrossOriginAndAllowsSameOrigin() {
        let base = URL(string: "https://api.example.com")!
        let delegate = OriginRestrictedRedirectDelegate(baseURL: base)
        let task = URLSession.shared.dataTask(with: base)
        let response = HTTPURLResponse(
            url: base.appendingPathComponent("v1"),
            statusCode: 302,
            httpVersion: "HTTP/1.1",
            headerFields: nil
        )!

        var crossOrigin: URLRequest?
        delegate.urlSession(
            URLSession.shared,
            task: task,
            willPerformHTTPRedirection: response,
            newRequest: URLRequest(url: URL(string: "https://attacker.example/collect")!),
            completionHandler: { crossOrigin = $0 }
        )
        XCTAssertNil(crossOrigin)

        var sameOrigin: URLRequest?
        let allowed = URLRequest(url: URL(string: "https://api.example.com/redirected")!)
        delegate.urlSession(
            URLSession.shared,
            task: task,
            willPerformHTTPRedirection: response,
            newRequest: allowed,
            completionHandler: { sameOrigin = $0 }
        )
        XCTAssertEqual(sameOrigin?.url, allowed.url)
        task.cancel()
    }
}
