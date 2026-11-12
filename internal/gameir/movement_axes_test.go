package gameir

import (
	"encoding/json"
	"strings"
	"testing"
)

func movementAxesFixture(t *testing.T) *Document {
	t.Helper()
	doc, err := DecodeAndValidate([]byte(v12Doc(t)))
	if err != nil {
		t.Fatal(err)
	}
	doc.Capabilities = append(doc.Capabilities, "input.movement_axes.v1")
	doc.Experience.Controls.MovementAxes = "x"
	return doc
}

func TestMovementAxesV1ValidSchemesAreExplicitAndVersioned(t *testing.T) {
	cases := []struct{ scheme, axes string }{
		{"drag_x", "x"},
		{"drag_free", "xy"},
		{"tap", "x"},
		{"tap", "xy"},
		{"lane_tap", "x"},
	}
	for _, tc := range cases {
		t.Run(tc.scheme+"_"+tc.axes, func(t *testing.T) {
			doc := movementAxesFixture(t)
			doc.Experience.Controls.Scheme = tc.scheme
			doc.Experience.Controls.MovementAxes = tc.axes
			if err := Validate(doc); err != nil {
				t.Fatalf("declared %s:%s rejected: %v", tc.scheme, tc.axes, err)
			}
			encoded, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), `"movementAxes":"`+tc.axes+`"`) {
				t.Fatal("canonical JSON dropped the declared movement plane")
			}
		})
	}
}

func TestMovementAxesV1MissingAndInconsistentDeclarationsFailClosed(t *testing.T) {
	cases := []struct {
		name, scheme, axes string
		dropCapability     bool
		irVersion          string
		want               string
	}{
		{"unsupported-z", "tap", "z", false, "", "must be x or xy"},
		{"no-capability", "tap", "xy", true, "", "requires input.movement_axes.v1"},
		{"missing-axes", "tap", "", false, "", "must be x or xy"},
		{"drag-x-claims-xy", "drag_x", "xy", false, "", "must be x"},
		{"drag-free-claims-x", "drag_free", "x", false, "", "must be xy"},
		{"lane-tap-claims-xy", "lane_tap", "xy", false, "", "must be x"},
		{"tap-target-has-no-player-movement", "tap_target", "xy", false, "", "cannot declare"},
		{"wrong-ir-version", "tap", "xy", false, "1.1", "requires Game IR 1.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := movementAxesFixture(t)
			doc.Experience.Controls.Scheme = tc.scheme
			doc.Experience.Controls.MovementAxes = tc.axes
			if tc.irVersion != "" {
				doc.IRVersion = tc.irVersion
			}
			if tc.dropCapability {
				doc.Capabilities = doc.Capabilities[:len(doc.Capabilities)-1]
			}
			if err := Validate(doc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestMovementAxesLegacyPackagesRemainByteCompatibleWhenAbsent(t *testing.T) {
	doc, err := DecodeAndValidate([]byte(v12Doc(t)))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("published legacy contract unexpectedly rejected: %v", err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "movementAxes") {
		t.Fatal("optional movementAxes must be absent from existing canonical Game IR hashes")
	}
}
