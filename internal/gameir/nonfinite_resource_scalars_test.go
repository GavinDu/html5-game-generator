package gameir

import (
	"math"
	"testing"
)

func TestGameIRRejectsNonFiniteSceneAndRenderScalars(t *testing.T) {
	// A generated Document can be validated *before* JSON serialization.
	// Range checks without finite() admit NaN because neither comparison
	// evaluates true, potentially poisoning physics or the Pixi renderer.
	cases := []struct {
		name  string
		valid float64
		set   func(*Document, *float64)
	}{
		{"pixels-per-meter", 100, func(d *Document, v *float64) {
			d.Scenes[0].PixelsPerMeter = v
		}},
		{"layer-alpha", 0.5, func(d *Document, v *float64) {
			d.Scenes[0].Layers[0].Alpha = v
		}},
		{"render-alpha", 0.5, func(d *Document, v *float64) {
			d.Scenes[0].Entities[0].Render.Alpha = v
		}},
		{"shape-stroke-width", 2, func(d *Document, v *float64) {
			d.Scenes[0].Entities[4].Render.Shape.StrokeWidth = v
		}},
		{"shape-corner-radius", 8, func(d *Document, v *float64) {
			d.Scenes[0].Entities[4].Render.Shape.CornerRadius = v
		}},
		{"text-font-size", 22, func(d *Document, v *float64) {
			d.Scenes[0].Entities[5].Render.Text.FontSize = v
		}},
		{"filter-scale", 1.5, func(d *Document, v *float64) {
			d.Scenes[0].Entities[2].Render.Filters[0].Scale = v
		}},
		{"emitter-start-alpha", 0.7, func(d *Document, v *float64) {
			d.Scenes[0].Emitters[0].StartAlpha = v
		}},
		{"emitter-end-alpha", 0.4, func(d *Document, v *float64) {
			d.Scenes[0].Emitters[0].EndAlpha = v
		}},
		{"joint-motor-max-force", 80, func(d *Document, v *float64) {
			d.Scenes[0].Joints[0].Motor = &JointMotor{
				Mode: "velocity", Target: 2, MaxForce: v,
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			valid, err := DecodeAndValidate([]byte(v11Doc))
			if err != nil {
				t.Fatalf("v1.1 reference fixture invalid: %v", err)
			}
			v := tc.valid
			tc.set(valid, &v)
			if err := Validate(valid); err != nil {
				t.Fatalf("finite value within bounds rejected: %v", err)
			}
			for _, invalid := range []struct {
				name   string
				number float64
			}{
				{"nan", math.NaN()},
				{"positive-infinity", math.Inf(1)},
				{"negative-infinity", math.Inf(-1)},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					d, err := DecodeAndValidate([]byte(v11Doc))
					if err != nil {
						t.Fatalf("v1.1 reference fixture invalid: %v", err)
					}
					value := invalid.number
					tc.set(d, &value)
					if err := Validate(d); err == nil {
						t.Fatalf("invalid non-finite %s accepted", tc.name)
					}
				})
			}
		})
	}
}
