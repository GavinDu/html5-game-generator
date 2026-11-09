package pipeline

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapTitlePreservesUTF8WhenByteBudgetEndsInsideChineseCharacter(t *testing.T) {
	input := "Remix: " + strings.Repeat("心", 40)
	title := capTitle(input, 80)
	if !utf8.ValidString(title) {
		t.Fatalf("remix title became invalid UTF-8: %q", title)
	}
	if len(title) > 80 || title != input[:79] {
		t.Fatalf("expected a valid 79-byte prefix within 80-byte budget, got %q (%d bytes)", title, len(title))
	}
}

func TestCapTitleEmojiAndInvalidBytes(t *testing.T) {
	for _, tc := range []struct {
		input string
		budget int
		want string
	}{
		{input: "🎮", budget: 3, want: ""},
		{input: "🎮", budget: 4, want: "🎮"},
		{input: "Go 🎮", budget: 5, want: "Go"},
		{input: "  Hello world  ", budget: 8, want: "Hello wo"},
		{input: "abc", budget: 0, want: ""},
		{input: "\xffabc", budget: 5, want: "abc"},
	} {
		got := capTitle(tc.input, tc.budget)
		if got != tc.want || !utf8.ValidString(got) || len(got) > tc.budget && tc.budget > 0 {
			t.Errorf("capTitle(%q, %d) = %q, want %q (valid UTF-8 inside budget)", tc.input, tc.budget, got, tc.want)
		}
	}
}
