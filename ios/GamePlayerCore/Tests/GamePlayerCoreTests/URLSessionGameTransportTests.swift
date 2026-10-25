import Foundation
import XCTest
@testable import GamePlayerCore

private final class CrossOriginRedirectURLProtocol: URLProtocol {
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let redirected = URL(string: "https://attacker.example/redirected")!
        let response = HTTPURLResponse(
            url: redirected,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data("{}".utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

private final class FixedBodyURLProtocol: URLProtocol {
    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let body = Data("payload".utf8)
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: 200,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Length": String(body.count)]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

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

    func testRejectsOversizedResponseBeforeBufferingEntireBody() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [FixedBodyURLProtocol.self]
        let session = URLSession(configuration: configuration)
        let transport = try URLSessionGameTransport(
            baseURL: URL(string: "https://api.example.com")!,
            session: session,
            packageByteLimit: 3
        )
        do {
            _ = try await transport.fetchPackage(gameID: "game-1", version: nil)
            XCTFail("response above configured limit must be rejected")
        } catch let error as GameTransportError {
            XCTAssertEqual(error, .responseTooLarge(limit: 3))
        }
        session.invalidateAndCancel()
    }

    func testAcceptsResponseExactlyAtConfiguredLimit() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [FixedBodyURLProtocol.self]
        let session = URLSession(configuration: configuration)
        let transport = try URLSessionGameTransport(
            baseURL: URL(string: "https://api.example.com")!,
            session: session,
            packageByteLimit: 7
        )
        let data = try await transport.fetchPackage(gameID: "game-1", version: nil)
        XCTAssertEqual(data, Data("payload".utf8))
        session.invalidateAndCancel()
    }

    func testRedirectPolicyAllowsOnlySameOriginTargets() {
        let base = URL(string: "https://api.example.com/v1")!
        let delegate = OriginRestrictedRedirectDelegate(baseURL: base)
        let task = URLSession.shared.dataTask(with: base)
        let response = HTTPURLResponse(
            url: base,
            statusCode: 302,
            httpVersion: "HTTP/1.1",
            headerFields: nil
        )!

        var sameOrigin: URLRequest?
        delegate.urlSession(
            URLSession.shared,
            task: task,
            willPerformHTTPRedirection: response,
            newRequest: URLRequest(url: URL(string: "https://api.example.com/next")!)
        ) { sameOrigin = $0 }
        XCTAssertNotNil(sameOrigin)

        var crossOrigin: URLRequest?
        delegate.urlSession(
            URLSession.shared,
            task: task,
            willPerformHTTPRedirection: response,
            newRequest: URLRequest(url: URL(string: "https://attacker.example/collect")!)
        ) { crossOrigin = $0 }
        XCTAssertNil(crossOrigin)
    }

    func testRejectsCrossOriginRedirectBeforeAcceptingResponseBytes() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [CrossOriginRedirectURLProtocol.self]
        let session = URLSession(configuration: configuration)
        let transport = try URLSessionGameTransport(
            baseURL: URL(string: "https://api.example.com")!,
            session: session
        )
        do {
            _ = try await transport.fetchPackage(gameID: "game-1", version: nil)
            XCTFail("cross-origin redirect must be rejected")
        } catch let error as GameTransportError {
            XCTAssertEqual(error, .rejectedURL)
        }
    }
}
