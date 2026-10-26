package api

import "testing"

func TestIsOpaqueCursor(t *testing.T) {
	// base64url({"v":1,"k":["x"],"f":"0123456789abcdef"})
	opaque := "eyJ2IjoxLCJrIjpbIngiXSwiZiI6IjAxMjM0NTY3ODlhYmNkZWYifQ"
	if !isOpaqueCursor(opaque) {
		t.Fatalf("store-shaped cursor: isOpaqueCursor = false, want true")
	}
	for _, raw := range []string{"comment-123", "not-base64!!!", ""} {
		if isOpaqueCursor(raw) {
			t.Fatalf("cursor %q: isOpaqueCursor = true, want false", raw)
		}
	}
}
