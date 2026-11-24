package attestation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// This is the wire grammar of the narrow diagnostic start-score profile,
// not a replacement for the general Game IR decoder or publisher validation.
// Object paths are explicit so Go's case-insensitive struct matching cannot
// reinterpret a property that the installed JavaScript runtime ignores.
func pinnedStartScoreWireKeys(path string) string {
	switch path {
	case "":
		return "irVersion capabilities game assets scenes inputBindings rules experience metadata"
	case ".game":
		return "id title description entryScene orientation sessionTargetSeconds audience"
	case ".metadata":
		return "creatorDisplayName parentGameId parentVersion tags"
	case ".scenes[]":
		return "id backgroundAssetId backgroundColor gravity worldSize pixelsPerMeter layers entities templates joints emitters"
	case ".rules[]":
		return "id when conditions actions"
	case ".rules[].when":
		return "event source target inputAction value"
	case ".rules[].actions[]":
		return "type target variable value assetId sceneId durationMs"
	default:
		return ""
	}
}

func validatePinnedStartScoreWire(raw []byte) error {
	if len(raw) == 0 || len(raw) > maxPinnedStartScoreIRBytes || !utf8.Valid(raw) {
		return errors.New("pinned diagnostic IR wire size or encoding invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanPinnedStartScoreWire(decoder, "", 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("pinned diagnostic IR has trailing or malformed data")
	}
	return nil
}

func scanPinnedStartScoreWire(decoder *json.Decoder, path string, depth int) error {
	if depth > 8 {
		return errors.New("pinned diagnostic IR wire nesting exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("pinned diagnostic IR wire token invalid")
	}
	switch token {
	case json.Delim('{'):
		allowed := strings.Fields(pinnedStartScoreWireKeys(path))
		if len(allowed) == 0 {
			return errors.New("pinned diagnostic IR object outside profile")
		}
		seen := make(map[string]bool, len(allowed))
		for decoder.More() {
			member, err := decoder.Token()
			key, ok := member.(string)
			if err != nil || !ok {
				return errors.New("pinned diagnostic IR member invalid")
			}
			known := false
			for _, canonical := range allowed {
				if key == canonical {
					known = true
					break
				}
			}
			if !known || seen[key] {
				return errors.New("pinned diagnostic IR noncanonical or duplicate member")
			}
			seen[key] = true
			if err := scanPinnedStartScoreWire(decoder, path+"."+key, depth+1); err != nil {
				return err
			}
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
			return errors.New("pinned diagnostic IR object malformed")
		}
	case json.Delim('['):
		for decoder.More() {
			if err := scanPinnedStartScoreWire(decoder, path+"[]", depth+1); err != nil {
				return err
			}
		}
		if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
			return errors.New("pinned diagnostic IR array malformed")
		}
	case json.Delim('}'), json.Delim(']'):
		return errors.New("pinned diagnostic IR unexpected closing delimiter")
	}
	return nil
}
