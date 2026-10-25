package socialgraph

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Page is the store-level pagination request. Limit 0 means the method
// default; a limit above the method maximum is an error, never clamped.
// Cursor "" means the first page.
type Page struct {
	Limit  int
	Cursor string
}

// IDPage is one page of an id list.
type IDPage struct {
	IDs        []string
	NextCursor string
}

// CommentPage is one page of comments.
type CommentPage struct {
	Comments   []Comment
	NextCursor string
}

// ReportPage is one page of moderation reports.
type ReportPage struct {
	Reports    []Report
	NextCursor string
}

type cursorPayload struct {
	Version int      `json:"v"`
	Keys    []string `json:"k"`
	Filter  string   `json:"f"`
}

func filterFingerprint(filter string) string {
	sum := sha256.Sum256([]byte(filter))
	return hex.EncodeToString(sum[:])[:16]
}

// encodeCursor builds an opaque, versioned cursor from the keyset keys
// of the last row returned and the canonical filter for the query. The
// cursor carries no server secret; it is untrusted input on decode and
// is only ever used as a query parameter, never interpolated into SQL.
func encodeCursor(keys []string, filter string) (string, error) {
	if len(keys) == 0 {
		return "", errors.New("cursor keys must not be empty")
	}
	for _, key := range keys {
		if len(key) > 2000 {
			return "", errors.New("cursor key is too long")
		}
	}
	raw, err := json.Marshal(cursorPayload{Version: 1, Keys: keys, Filter: filterFingerprint(filter)})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// decodeCursor validates an opaque cursor against the current query
// filter and returns its keyset keys. A cursor reused across different
// filter values is rejected: without that, changing a filter mid-walk
// silently skips rows that match the new filter but sort before the
// cursor.
func decodeCursor(encoded, filter string) ([]string, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, errors.New("cursor must not be empty")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("cursor is not valid base64")
	}
	var payload cursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("cursor is not valid JSON")
	}
	if payload.Version != 1 {
		return nil, errors.New("unsupported cursor version")
	}
	if len(payload.Keys) == 0 {
		return nil, errors.New("cursor has no keys")
	}
	if payload.Filter != filterFingerprint(filter) {
		return nil, errors.New("cursor does not match this query")
	}
	for _, key := range payload.Keys {
		if len(key) > 2000 {
			return nil, errors.New("cursor key is too long")
		}
	}
	if len(payload.Keys) == 2 {
		if _, err := time.Parse(time.RFC3339Nano, payload.Keys[0]); err != nil {
			return nil, errors.New("cursor timestamp is invalid")
		}
	}
	return payload.Keys, nil
}

// resolveIDPageLimit applies the public-list limit contract: default 20,
// maximum 50, out-of-range is an error, never clamped.
func resolveIDPageLimit(limit int) (int, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 1 || limit > 50 {
		return 0, errors.New("limit must be between 1 and 50")
	}
	return limit, nil
}

// resolveReportPageLimit applies the operator-list limit contract:
// default 100, maximum 500, matching the existing report handlers.
func resolveReportPageLimit(limit int) (int, error) {
	if limit == 0 {
		return 100, nil
	}
	if limit < 1 || limit > 500 {
		return 0, errors.New("limit must be between 1 and 500")
	}
	return limit, nil
}

func validateID(id string) error {
	if strings.TrimSpace(id) == "" || len(id) > 128 {
		return errors.New("id must be 1–128 characters")
	}
	return nil
}
