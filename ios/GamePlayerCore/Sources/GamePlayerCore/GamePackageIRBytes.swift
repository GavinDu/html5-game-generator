import Foundation

/// Extracts the exact JSON bytes of the top-level `ir` field in a backend
/// Game Package v2.0 response. Go hashes json.Marshal(GameIR) and writes the
/// same compact bytes into the enclosing json.Marshal(GamePackage). Re-encoding
/// through Swift dictionaries would reorder object keys and produce false hash
/// mismatches; hashing the original field bytes avoids that mistake.
///
/// This is intentionally only a structural byte scanner, not an independent
/// JSON parser. PlayerContract first decodes the complete envelope with
/// JSONDecoder; this scanner locates a unique top-level field and its byte
/// range. Any ambiguity, duplicate `ir` field or mismatched delimiter fails
/// closed instead of accepting a differently interpreted document.
enum GamePackageIRBytes {
    static func extract(from data: Data) -> Data? {
        let bytes = Array(data)
        var cursor = 0
        skipWhitespace(bytes, &cursor)
        guard consume(123, in: bytes, at: &cursor) else { return nil } // {

        var found: Range<Int>?
        skipWhitespace(bytes, &cursor)
        if consume(125, in: bytes, at: &cursor) { return nil }

        while cursor < bytes.count {
            skipWhitespace(bytes, &cursor)
            guard let field = scanString(bytes, &cursor) else { return nil }
            skipWhitespace(bytes, &cursor)
            guard consume(58, in: bytes, at: &cursor) else { return nil } // :
            skipWhitespace(bytes, &cursor)
            guard let value = scanValue(bytes, &cursor) else { return nil }
            if field == "ir" {
                guard found == nil, bytes[value.lowerBound] == 123 else { return nil }
                found = value
            }

            skipWhitespace(bytes, &cursor)
            if consume(125, in: bytes, at: &cursor) { // }
                skipWhitespace(bytes, &cursor)
                guard cursor == bytes.count, let range = found else { return nil }
                return Data(bytes[range])
            }
            guard consume(44, in: bytes, at: &cursor) else { return nil } // ,
        }
        return nil
    }

    private static func skipWhitespace(_ bytes: [UInt8], _ cursor: inout Int) {
        while cursor < bytes.count && [9, 10, 13, 32].contains(bytes[cursor]) {
            cursor += 1
        }
    }

    private static func consume(_ expected: UInt8, in bytes: [UInt8], at cursor: inout Int) -> Bool {
        guard cursor < bytes.count, bytes[cursor] == expected else { return false }
        cursor += 1
        return true
    }

    private static func scanString(_ bytes: [UInt8], _ cursor: inout Int) -> String? {
        let start = cursor
        guard consume(34, in: bytes, at: &cursor) else { return nil } // "
        var escaped = false
        while cursor < bytes.count {
            let b = bytes[cursor]
            cursor += 1
            if escaped {
                escaped = false
            } else if b == 92 { // backslash
                escaped = true
            } else if b == 34 { // closing "
                return try? JSONDecoder().decode(String.self, from: Data(bytes[start..<cursor]))
            }
        }
        return nil
    }

    private static func scanValue(_ bytes: [UInt8], _ cursor: inout Int) -> Range<Int>? {
        guard cursor < bytes.count else { return nil }
        let start = cursor

        if bytes[cursor] == 34 { // JSON string
            guard scanString(bytes, &cursor) != nil else { return nil }
            return start..<cursor
        }

        if bytes[cursor] == 123 || bytes[cursor] == 91 { // object or array
            var closers: [UInt8] = [bytes[cursor] == 123 ? 125 : 93]
            cursor += 1
            var inString = false
            var escaped = false
            while cursor < bytes.count {
                let b = bytes[cursor]
                cursor += 1
                if inString {
                    if escaped {
                        escaped = false
                    } else if b == 92 {
                        escaped = true
                    } else if b == 34 {
                        inString = false
                    }
                    continue
                }
                switch b {
                case 34: inString = true
                case 123: closers.append(125)
                case 91: closers.append(93)
                case 125, 93:
                    guard closers.popLast() == b else { return nil }
                    if closers.isEmpty { return start..<cursor }
                default: break
                }
            }
            return nil
        }

        while cursor < bytes.count && ![44, 93, 125, 9, 10, 13, 32].contains(bytes[cursor]) {
            cursor += 1
        }
        return cursor > start ? start..<cursor : nil
    }
}
