import Foundation

/// A validated route into a specific game package. Optional version and remix
/// lineage are preserved from share links so clients can open the intended edition.
public struct GameDeepLink: Equatable, Sendable {
    public let gameID: String
    public let version: String?
    public let remixParentID: String?

    public init(gameID: String, version: String? = nil, remixParentID: String? = nil) {
        self.gameID = gameID
        self.version = version
        self.remixParentID = remixParentID
    }
}

public enum GameDeepLinkError: Error, Equatable {
    case unsupportedURL
    case invalidGameID
    case invalidVersion
    case invalidRemixParentID
    case duplicateQueryParameter
}

public struct GameDeepLinkRouter: Sendable {
    private let allowedHosts: Set<String>
    private let customScheme: String

    /// `allowedHosts` should contain the app's canonical universal-link hosts.
    public init(allowedHosts: Set<String>, customScheme: String = "gameplayer") {
        self.allowedHosts = Set(allowedHosts.map { $0.lowercased() })
        self.customScheme = customScheme.lowercased()
    }

    public func route(_ url: URL) throws -> GameDeepLink {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let scheme = components.scheme?.lowercased() else {
            throw GameDeepLinkError.unsupportedURL
        }

        let segments: [String]
        if scheme == "https" {
            guard let host = components.host?.lowercased(), allowedHosts.contains(host),
                  components.user == nil, components.password == nil,
                  (components.port == nil || components.port == 443),
                  components.fragment == nil else {
                throw GameDeepLinkError.unsupportedURL
            }
            segments = components.path.split(separator: "/").map(String.init)
        } else if scheme == customScheme {
            let hostSegments = components.host.map { [$0] } ?? []
            segments = hostSegments + components.path.split(separator: "/").map(String.init)
        } else {
            throw GameDeepLinkError.unsupportedURL
        }

        guard segments.count == 2, segments[0] == "games" else {
            throw GameDeepLinkError.unsupportedURL
        }
        let gameID = segments[1]
        guard Self.validIdentifier(gameID) else { throw GameDeepLinkError.invalidGameID }

        let queryItems = components.queryItems ?? []
        let parentKeys: Set<String> = ["parentGameId", "remixParent", "remix_parent"]
        let recognizedKeys = queryItems.filter { $0.name == "version" || parentKeys.contains($0.name) }
        let recognizedNames = recognizedKeys.map(\.name)
        guard Set(recognizedNames).count == recognizedNames.count,
              recognizedNames.filter({ parentKeys.contains($0) }).count <= 1 else {
            throw GameDeepLinkError.duplicateQueryParameter
        }
        let query = Dictionary(
            queryItems.compactMap { item -> (String, String)? in
                guard let value = item.value else { return nil }
                return (item.name, value)
            },
            uniquingKeysWith: { first, _ in first }
        )
        let version = query["version"]
        let parent = query["parentGameId"] ?? query["remixParent"] ?? query["remix_parent"]
        if let version, !Self.validIdentifier(version) { throw GameDeepLinkError.invalidVersion }
        if let parent, !Self.validIdentifier(parent) { throw GameDeepLinkError.invalidRemixParentID }
        return GameDeepLink(gameID: gameID, version: version, remixParentID: parent)
    }

    /// Creates a stable HTTPS share URL; the server must still enforce access control.
    public func shareURL(for route: GameDeepLink, canonicalHost: String) throws -> URL {
        guard allowedHosts.contains(canonicalHost.lowercased()), Self.validIdentifier(route.gameID) else {
            throw GameDeepLinkError.unsupportedURL
        }
        var components = URLComponents()
        components.scheme = "https"
        components.host = canonicalHost.lowercased()
        components.path = "/games/\(route.gameID)"
        var items = [URLQueryItem]()
        if let version = route.version {
            guard Self.validIdentifier(version) else { throw GameDeepLinkError.invalidVersion }
            items.append(URLQueryItem(name: "version", value: version))
        }
        if let parent = route.remixParentID {
            guard Self.validIdentifier(parent) else { throw GameDeepLinkError.invalidRemixParentID }
            items.append(URLQueryItem(name: "parentGameId", value: parent))
        }
        components.queryItems = items.isEmpty ? nil : items
        guard let url = components.url else { throw GameDeepLinkError.unsupportedURL }
        return url
    }

    private static func validIdentifier(_ value: String) -> Bool {
        guard !value.isEmpty, value.utf8.count <= 128 else { return false }
        return value.unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._~-").contains($0)
        }
    }
}
