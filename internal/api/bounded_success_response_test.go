package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoundedPreencodedSuccessResponseExactByteBudget(t *testing.T) {
	const secret = "private-owner-game-id-with-credential-token"
	encoded := []byte(`{"gameId":"` + secret + `","packageFormatVersion":"2.0"}`)
	for _, test := range []struct {
		name      string
		maxBytes  int
		wantAllow bool
	}{
		{"exact_limit", len(encoded), true},
		{"room_for_one_more", len(encoded) + 1, true},
		{"limit_minus_one", len(encoded) - 1, false},
		{"one_byte_beyond_budget", len(encoded) - 1, false},
		{"zero_budget", 0, false},
		{"negative_budget", -1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Vary", "Authorization, X-User-ID")
			rec.Header().Set("Cache-Control", "public, max-age=31536000")
			rec.Header().Set("ETag", `"published-game-v2"`)
			rec.Header().Set("Last-Modified", "Wed, 01 Jan 2025 00:00:00 GMT")
			rec.Header().Set("Expires", "Wed, 01 Jan 2031 00:00:00 GMT")
			if !test.wantAllow {
				rec.Header().Set("Content-Length", "999999")
				rec.Header().Set("Content-Encoding", "gzip")
			}

			if allowed := writeBoundedPreencodedJSONSuccess(
				rec, encoded, test.maxBytes,
			); allowed != test.wantAllow {
				t.Fatalf("admitted=%t, want %t", allowed, test.wantAllow)
			}
			if got := rec.Header().Get("Vary"); got != "Authorization, X-User-ID" {
				t.Fatalf("Vary stripped: %q", got)
			}
			if test.wantAllow {
				if rec.Code != http.StatusOK {
					t.Fatalf("status=%d, want 200", rec.Code)
				}
				if !bytes.Equal(rec.Body.Bytes(), encoded) {
					t.Fatalf("success body changed")
				}
				if rec.Header().Get("ETag") != `"published-game-v2"` ||
					rec.Header().Get("Cache-Control") != "public, max-age=31536000" {
					t.Fatal("successful response cache/digest contract changed")
				}
				if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
					t.Fatal("successful document has incorrect content type")
				}
				return
			}

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("denied status=%d, want 422 (never 200)", rec.Code)
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("denied document must never be public-cached")
			}
			for _, header := range []string{
				"ETag", "Last-Modified", "Expires", "Content-Length", "Content-Encoding",
			} {
				if got := rec.Header().Get(header); got != "" {
					t.Errorf("stale %s header disclosed on denial: %q", header, got)
				}
			}
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatal("denial revealed private GamePackage content")
			}
			var body struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("denial must be valid safe JSON: %v", err)
			}
			if body.Error != "successful_document_size_rejected" ||
				body.Message != "published document could not be delivered" {
				t.Fatalf("denial must have a fixed safe category: %+v", body)
			}
		})
	}
}

func TestBoundedPreencodedSuccessRejectsEmptyDocumentBeforeSendingOK(t *testing.T) {
	for _, test := range []struct {
		name    string
		encoded []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Cache-Control", "public, immutable")
			if writeBoundedPreencodedJSONSuccess(rec, test.encoded, 64) {
				t.Fatal("empty JSON incorrectly admitted")
			}
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("empty successful JSON must be rejected before 200, got %d", rec.Code)
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("empty document denial remains cacheable")
			}
		})
	}
}
