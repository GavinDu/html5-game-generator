package socialgraph

import (
	"strings"
	"testing"
	"time"
)

func TestCursorRoundTripAndFingerprint(t *testing.T) {
	encoded, err := encodeCursor([]string{"creator-1"}, "following:viewer-1")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := decodeCursor(encoded, "following:viewer-1")
	if err != nil || len(keys) != 1 || keys[0] != "creator-1" {
		t.Fatalf("decode cursor = %v err=%v, want [creator-1]", keys, err)
	}
	// Same cursor, different filter: rejected.
	if _, err := decodeCursor(encoded, "following:viewer-2"); err == nil {
		t.Fatalf("decode cursor with different filter: got no error, want fingerprint mismatch")
	}
	// Timestamp key round-trips.
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err = encodeCursor([]string{ts, "comment-1"}, "comments:game-1")
	if err != nil {
		t.Fatal(err)
	}
	keys, err = decodeCursor(encoded, "comments:game-1")
	if err != nil || len(keys) != 2 || keys[0] != ts || keys[1] != "comment-1" {
		t.Fatalf("decode timestamp cursor = %v err=%v", keys, err)
	}
}

func TestCursorRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		filter  string
	}{
		{"not base64", "!!!not-base64!!!", "following:v"},
		{"bad json", "aGVsbG8", "following:v"},
		{"wrong version", "eyJ2IjoyLCJrIjpbIngiXSwiZiI6IjEyMzQ1Njc4OTBhYmNkZWYifQ", "following:v"},
		{"over-long key", "", "following:v"},
	}
	// Over-long key case: build a real cursor with a 2001-char key.
	overlong, err := encodeCursor([]string{strings.Repeat("x", 2001)}, "following:v")
	if err == nil {
		cases[3].encoded = overlong
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeCursor(tc.encoded, tc.filter); err == nil {
				t.Fatalf("decode %q: got no error, want an error", tc.encoded)
			}
		})
	}
}

func TestPageLimits(t *testing.T) {
	if _, err := resolveIDPageLimit(51); err == nil {
		t.Fatalf("id page limit 51: got no error, want an error")
	}
	if n, err := resolveIDPageLimit(0); err != nil || n != 20 {
		t.Fatalf("id page default = %d err=%v, want 20", n, err)
	}
	if _, err := resolveReportPageLimit(501); err == nil {
		t.Fatalf("report page limit 501: got no error, want an error")
	}
	if n, err := resolveReportPageLimit(0); err != nil || n != 100 {
		t.Fatalf("report page default = %d err=%v, want 100", n, err)
	}
}
