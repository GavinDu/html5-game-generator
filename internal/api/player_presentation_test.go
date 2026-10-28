package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestPlayerPlayPresentationContract(t *testing.T) {
	srv := newTestServer()
	rec := do(t, srv, http.MethodGet, "/player?game=sample-rainbow-bear&presentation=play", nil, "viewer")
	if rec.Code != http.StatusOK {
		t.Fatalf("play presentation: %d %s", rec.Code, rec.Body.String())
	}

	for _, fragment := range []string{
		"body.dq-play",
		"presentation=play",
		"document.getElementById('account')?.remove()",
		"section[aria-labelledby=\"feed-title\"]",
		"if (!playPresentation) initializeAccountControls();",
	} {
		if !strings.Contains(playerHTML+pixiRuntimeJS, fragment) {
			t.Fatalf("play presentation contract missing %q", fragment)
		}
	}

	if strings.Contains(rec.Body.String(), "data-presentation=\"browser\"") {
		t.Fatal("browser presentation marker leaked into play response")
	}
}
