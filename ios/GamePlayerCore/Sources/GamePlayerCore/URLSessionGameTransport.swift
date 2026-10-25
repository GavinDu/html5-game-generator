import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum GameTransportError: Error, Equatable {
    case invalidBaseURL
    case rejectedURL
    case invalidResponse
    case httpStatus(Int)
    case responseTooLarge(limit: Int)
}

/// Blocks redirects outside the configured origin before redirected requests are sent.
final class OriginRestrictedRedirectDelegate: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    private let scheme: String
    private let host: String
    private let port: Int

    init(baseURL: URL) {
        scheme = baseURL.scheme?.lowercased() ?? ""
        host = baseURL.host?.lowercased() ?? ""
        port = Self.effectivePort(baseURL)
    }

    func urlSession(
        _ session: URLSession,
        task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void
    ) {
        guard let url = newRequest.url,
              url.scheme?.lowercased() == scheme,
              url.host?.lowercased() == host,
              Self.effectivePort(url) == port,
              url.user == nil, url.password == nil else {
            completionHandler(nil)
            return
        }
        completionHandler(newRequest)
    }

    private static func effectivePort(_ url: URL) -> Int {
        if let port = url.port { return port }
        return url.scheme?.lowercased() == "https" ? 443 : 80
    }
}

/// URLSession-backed adapters for the launch coordinator. Package and asset URLs
/// are restricted to the configured API origin. Response bodies are streamed
/// under a byte budget, and cross-origin redirects are rejected. Auth headers
/// are injected by the app.
public struct URLSessionGameTransport: GamePackageTransport, GameAssetTransport, Sendable {
    private let baseURL: URL
    private let session: URLSession
    private let redirectDelegate: OriginRestrictedRedirectDelegate?
    private let headers: @Sendable () async -> [String: String]
    private let packageByteLimit: Int
    private let assetByteLimit: Int

    public init(
        baseURL: URL,
        session: URLSession? = nil,
        packageByteLimit: Int = 10 * 1024 * 1024,
        assetByteLimit: Int = 25 * 1024 * 1024,
        headers: @escaping @Sendable () async -> [String: String] = { [:] }
    ) throws {
        guard Self.isAllowedTransportURL(baseURL), baseURL.host != nil else {
            throw GameTransportError.invalidBaseURL
        }
        self.baseURL = baseURL
        if let session {
            self.session = session
            self.redirectDelegate = nil
        } else {
            let delegate = OriginRestrictedRedirectDelegate(baseURL: baseURL)
            self.redirectDelegate = delegate
            self.session = URLSession(configuration: .default, delegate: delegate, delegateQueue: nil)
        }
        self.packageByteLimit = max(1, packageByteLimit)
        self.assetByteLimit = max(1, assetByteLimit)
        self.headers = headers
    }

    public func fetchPackage(gameID: String, version: String?) async throws -> Data {
        guard Self.validIdentifier(gameID) else { throw GameTransportError.rejectedURL }
        var url = baseURL
            .appendingPathComponent("v1", isDirectory: true)
            .appendingPathComponent("games", isDirectory: true)
            .appendingPathComponent(gameID, isDirectory: true)
            .appendingPathComponent("package", isDirectory: false)
        if let version {
            guard Self.validIdentifier(version) else { throw GameTransportError.rejectedURL }
            guard var components = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
                throw GameTransportError.rejectedURL
            }
            components.queryItems = [URLQueryItem(name: "version", value: version)]
            guard let updated = components.url else { throw GameTransportError.rejectedURL }
            url = updated
        }
        return try await get(url, byteLimit: packageByteLimit)
    }

    public func fetchAsset(_ asset: PackageAsset) async throws -> Data {
        guard let url = URL(string: asset.url, relativeTo: baseURL)?.absoluteURL,
              Self.isAllowedTransportURL(url),
              url.host?.lowercased() == baseURL.host?.lowercased(),
              url.port == baseURL.port,
              url.user == nil, url.password == nil else {
            throw GameTransportError.rejectedURL
        }
        return try await get(url, byteLimit: assetByteLimit)
    }

    private func get(_ url: URL, byteLimit: Int) async throws -> Data {
        guard Self.isAllowedTransportURL(url),
              url.host?.lowercased() == baseURL.host?.lowercased(),
              url.port == baseURL.port else {
            throw GameTransportError.rejectedURL
        }
        var request = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 30)
        request.httpMethod = "GET"
        request.setValue("no-cache", forHTTPHeaderField: "Cache-Control")
        for (key, value) in await headers() {
            request.setValue(value, forHTTPHeaderField: key)
        }
        let (bytes, response) = try await session.bytes(for: request)
        guard let response = response as? HTTPURLResponse else { throw GameTransportError.invalidResponse }
        guard let finalURL = response.url,
              Self.isAllowedTransportURL(finalURL),
              finalURL.host?.lowercased() == baseURL.host?.lowercased(),
              Self.effectivePort(finalURL) == Self.effectivePort(baseURL) else {
            throw GameTransportError.rejectedURL
        }
        guard (200..<300).contains(response.statusCode) else { throw GameTransportError.httpStatus(response.statusCode) }

        var data = Data()
        data.reserveCapacity(min(byteLimit, 64 * 1024))
        for try await byte in bytes {
            guard data.count < byteLimit else {
                throw GameTransportError.responseTooLarge(limit: byteLimit)
            }
            data.append(byte)
        }
        return data
    }

    private static func isAllowedTransportURL(_ url: URL) -> Bool {
        guard let scheme = url.scheme?.lowercased(), let host = url.host, !host.isEmpty,
              url.user == nil, url.password == nil else { return false }
        if scheme == "https" { return true }
        if scheme == "http" {
            let normalized = host.lowercased()
            return normalized == "localhost" || normalized == "127.0.0.1" || normalized == "::1"
        }
        return false
    }

    private static func effectivePort(_ url: URL) -> Int {
        if let port = url.port { return port }
        return url.scheme?.lowercased() == "https" ? 443 : 80
    }

    private static func validIdentifier(_ value: String) -> Bool {
        guard !value.isEmpty, value.utf8.count <= 128 else { return false }
        return value.unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._~-").contains($0)
        }
    }
}
