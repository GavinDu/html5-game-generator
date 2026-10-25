import Foundation

/// Share metadata stays data-only. The parent identifies the source game when
/// this game is a remix; it is not an instruction to execute code from that game.
public struct GameShareLineage: Codable, Equatable, Sendable {
    public let gameID: String
    public let version: String
    public let parentGameID: String?

    public init(gameID: String, version: String, parentGameID: String? = nil) throws {
        guard Self.isSafeIdentifier(gameID), Self.isSafeIdentifier(version) else {
            throw GameShareLinkError.invalidIdentifier
        }
        if let parentGameID, !Self.isSafeIdentifier(parentGameID) {
            throw GameShareLinkError.invalidIdentifier
        }
        self.gameID = gameID
        self.version = version
        self.parentGameID = parentGameID
    }

    private static func isSafeIdentifier(_ value: String) -> Bool {
        !value.isEmpty &&
        value.count <= 200 &&
        value.rangeOfCharacter(from: .controlCharacters) == nil &&
        value.trimmingCharacters(in: .whitespacesAndNewlines) == value
    }
}

public enum GameShareLinkError: Error, Equatable, Sendable {
    case invalidBaseURL
    case invalidIdentifier
    case malformedURL
    case missingGameID
    case missingVersion
}

/// Produces and parses HTTPS game links without hard-coding the deployment
/// domain. The host app should supply the canonical public origin configured by
/// the backend; never accept an arbitrary origin from a game package.
public struct GameShareLinkBuilder: Sendable {
    private let baseURL: URL

    public init(baseURL: URL) throws {
        guard let scheme = baseURL.scheme?.lowercased(),
              scheme == "https",
              baseURL.host != nil,
              baseURL.user == nil,
              baseURL.password == nil,
              baseURL.query == nil,
              baseURL.fragment == nil else {
            throw GameShareLinkError.invalidBaseURL
        }
        self.baseURL = baseURL
    }

    public func makeLink(for lineage: GameShareLineage) throws -> URL {
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)
        guard components != nil else { throw GameShareLinkError.invalidBaseURL }
        let prefix = (components?.path ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let gamePath = "games/" + lineage.gameID
        components?.path = "/" + ([prefix, gamePath].filter { !$0.isEmpty }.joined(separator: "/"))
        var query = [
            URLQueryItem(name: "version", value: lineage.version)
        ]
        if let parent = lineage.parentGameID {
            query.append(URLQueryItem(name: "parentGameId", value: parent))
        }
        components?.queryItems = query
        guard let url = components?.url else { throw GameShareLinkError.malformedURL }
        return url
    }

    public func parse(_ url: URL) throws -> GameShareLineage {
        guard let incoming = URLComponents(url: url, resolvingAgainstBaseURL: false),
              incoming.scheme?.lowercased() == "https",
              incoming.host?.lowercased() == baseURL.host?.lowercased(),
              incoming.port == baseURL.port else {
            throw GameShareLinkError.malformedURL
        }
        let basePath = baseURL.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let incomingPath = incoming.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let prefix = basePath.isEmpty ? "" : basePath + "/"
        guard incomingPath.hasPrefix(prefix + "games/") else {
            throw GameShareLinkError.malformedURL
        }
        let gameID = String(incomingPath.dropFirst((prefix + "games/").count))
        guard !gameID.isEmpty, !gameID.contains("/") else {
            throw GameShareLinkError.missingGameID
        }
        let values = Dictionary(incoming.queryItems?.map { ($0.name, $0.value ?? "") } ?? [], uniquingKeysWith: { first, _ in first })
        guard let version = values["version"], !version.isEmpty else {
            throw GameShareLinkError.missingVersion
        }
        return try GameShareLineage(gameID: gameID, version: version, parentGameID: values["parentGameId"])
    }
}
