package replaytrace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"doodlequest/internal/gameir"
)

const (
	// MaxEncodedTraceBytes prevents a future HTTP handler from allocating
	// unbounded memory before semantic event-count admission.
	MaxEncodedTraceBytes = 1 << 20
	maxWireDepth         = 6
)

// DecodeStrictV2 converts untrusted JSON into the existing pure semantic
// trace grammar. It does not authenticate a publisher, authorize a game,
// bind the caller's full IR hash, or verify a score. A future route must
// perform those checks independently before trusting any resulting evidence.
func DecodeStrictV2(ir *gameir.Document, payload []byte) (Trace, error) {
	var empty Trace
	if len(payload) == 0 || len(payload) > MaxEncodedTraceBytes {
		return empty, errors.New("semantic trace JSON size outside bounds")
	}
	if !utf8.Valid(payload) {
		return empty, errors.New("semantic trace JSON contains invalid UTF-8")
	}

	// encoding/json substitutes U+FFFD for unpaired escaped UTF-16.
	// Reject that ambiguous source text before normalized action names can
	// match published IR and receive an otherwise valid canonical digest.
	if err := rejectUnpairedUnicodeEscapes(payload); err != nil {
		return empty, err
	}

	// A conventional json.Unmarshal silently keeps the final occurrence of
	// repeated object keys. Such ambiguous evidence must never canonicalize
	// into a different trace on different clients.
	keyDecoder := json.NewDecoder(bytes.NewReader(payload))
	keyDecoder.UseNumber()
	if err := scanUniqueJSON(keyDecoder, 0); err != nil {
		return empty, fmt.Errorf("semantic trace JSON grammar: %w", err)
	}
	if _, err := keyDecoder.Token(); err != io.EOF {
		if err == nil {
			return empty, errors.New("semantic trace JSON has trailing values")
		}
		return empty, fmt.Errorf("semantic trace JSON trailing data: %w", err)
	}

	// Check field presence/null separately: decoding JSON null into a Go
	// zero-value scalar or pointer would otherwise erase wire distinctions.
	fields, err := requireWireObject(payload, "schema", "durationTicks", "events")
	if err != nil {
		return empty, err
	}
	var eventRecords []json.RawMessage
	if err := json.Unmarshal(fields["events"], &eventRecords); err != nil {
		return empty, fmt.Errorf("semantic trace events must be an array: %w", err)
	}
	if len(eventRecords) > MaxEvents {
		return empty, errors.New("semantic trace exceeds event budget")
	}
	for i, record := range eventRecords {
		event, err := requireWireObject(record, "tick", "ordinal", "action", "device", "phase")
		if err != nil {
			return empty, fmt.Errorf("semantic event %d: %w", i, err)
		}
		if position, ok := event["position"]; ok {
			if _, err := requireWireObject(position, "x", "y"); err != nil {
				return empty, fmt.Errorf("semantic event %d position: %w", i, err)
			}
		}
	}
	// The typed decoder checks all nested unknown-field exclusions.

	typedDecoder := json.NewDecoder(bytes.NewReader(payload))
	typedDecoder.DisallowUnknownFields()
	var trace Trace
	if err := typedDecoder.Decode(&trace); err != nil {
		return empty, fmt.Errorf("semantic trace fields: %w", err)
	}
	if err := Validate(ir, trace); err != nil {
		return empty, err
	}
	return trace, nil
}

func requireWireObject(payload json.RawMessage, required ...string) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("required JSON object missing or null")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, err
	}
	for _, name := range required {
		raw, ok := object[name]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("required JSON field %q missing or null", name)
		}
	}
	return object, nil
}

// scanUniqueJSON rejects duplicate decoded keys at any nested object depth.
// A small structural depth bound rules out pathological nesting before the
// typed decoder; the token scanner also rejects malformed delimiters.
func scanUniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > maxWireDepth {
		return errors.New("semantic trace JSON nesting too deep")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("non-string object member")
			}
			if _, present := seen[key]; present {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanUniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("malformed JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanUniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("malformed JSON array")
		}
	default:
		return errors.New("unexpected JSON closing delimiter")
	}
	return nil
}

// rejectUnpairedUnicodeEscapes preserves valid surrogate pairs and escaped
// literal backslashes, but rejects malformed Unicode *wire spelling* rather
// than allowing encoding/json to substitute the replacement rune.
func rejectUnpairedUnicodeEscapes(payload []byte) error {
	inString := false
	for i := 0; i < len(payload); i++ {
		switch payload[i] {
		case '"':
			inString = !inString
		case '\\':
			if !inString {
				continue // The JSON token decoder rejects invalid outside escapes.
			}
			if i+1 >= len(payload) {
				return errors.New("unterminated JSON string escape")
			}
			if payload[i+1] != 'u' {
				i++ // Escaped quote or backslash is not a Unicode escape.
				continue
			}
			if i+6 > len(payload) {
				return errors.New("incomplete escaped Unicode codepoint")
			}
			value, ok := readEscapedHexQuad(payload[i+2 : i+6])
			if !ok {
				return errors.New("invalid escaped Unicode codepoint")
			}
			i += 5 // Skip the complete \uXXXX spelling.
			switch {
			case value >= 0xD800 && value <= 0xDBFF:
				if i+6 >= len(payload) || payload[i+1] != '\\' || payload[i+2] != 'u' {
					return errors.New("unpaired high UTF-16 surrogate escape")
				}
				low, valid := readEscapedHexQuad(payload[i+3 : i+7])
				if !valid || low < 0xDC00 || low > 0xDFFF {
					return errors.New("high UTF-16 surrogate escape has no low surrogate")
				}
				i += 6 // Skip the complete following \uXXXX low surrogate.
			case value >= 0xDC00 && value <= 0xDFFF:
				return errors.New("unpaired low UTF-16 surrogate escape")
			}
		}
	}
	return nil // Syntax, quoting, and trailing values remain JSON decoder-owned.
}

func readEscapedHexQuad(src []byte) (uint16, bool) {
	if len(src) != 4 {
		return 0, false
	}
	var value uint16
	for _, digit := range src {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
