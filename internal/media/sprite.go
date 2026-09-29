// Package media generates bundle assets with the standard library only.
// The stub sprite painter stands in for the style-preserving AI polish
// stage: same interface (label in -> transparent PNG out), real model later.
package media

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
)

// SpritePNG paints a simple cute creature on a transparent background,
// tinted by a hash of the element label so each element is distinct.
func SpritePNG(label string) []byte {
	const S = 96
	img := image.NewNRGBA(image.Rect(0, 0, S, S))

	h := fnv.New32a()
	h.Write([]byte(label))
	hue := float64(h.Sum32()%360) / 360.0
	body := hsv(hue, 0.65, 0.95)
	dark := hsv(hue, 0.7, 0.75)
	light := hsv(hue, 0.35, 1.0)

	// Body and head.
	fillCircle(img, 48, 58, 30, body)
	fillCircle(img, 48, 30, 19, body)
	// Belly patch.
	fillCircle(img, 48, 62, 17, light)
	// Spikes along the back.
	for i := 0; i < 3; i++ {
		fillCircle(img, 22+8*i, 34-8*i, 6, dark)
	}
	// Eyes.
	fillCircle(img, 41, 28, 6, color.NRGBA{255, 255, 255, 255})
	fillCircle(img, 55, 28, 6, color.NRGBA{255, 255, 255, 255})
	fillCircle(img, 42, 29, 3, color.NRGBA{20, 20, 30, 255})
	fillCircle(img, 56, 29, 3, color.NRGBA{20, 20, 30, 255})
	// Smile.
	for x := 40; x <= 56; x++ {
		y := 38 + int(3*math.Sin(float64(x-40)/16*math.Pi))
		img.SetNRGBA(x, y, color.NRGBA{40, 20, 20, 255})
		img.SetNRGBA(x, y+1, color.NRGBA{40, 20, 20, 255})
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// StarPNG paints the collectible star used by the stub game template.
func StarPNG() []byte {
	const S = 48
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	gold := color.NRGBA{255, 205, 60, 255}
	cx, cy := 24.0, 24.0
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			r := math.Hypot(dx, dy)
			a := math.Atan2(dy, dx)
			// 5-point star radius function.
			edge := 20 * (0.72 + 0.28*math.Abs(math.Cos(2.5*a+math.Pi/2)))
			if r <= edge {
				img.SetNRGBA(x, y, gold)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func fillCircle(img *image.NRGBA, cx, cy, r int, c color.NRGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				if image.Pt(x, y).In(img.Bounds()) {
					img.SetNRGBA(x, y, c)
				}
			}
		}
	}
}

func hsv(h, s, v float64) color.NRGBA {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h*6, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 1.0/6:
		r, g, b = c, x, 0
	case h < 2.0/6:
		r, g, b = x, c, 0
	case h < 3.0/6:
		r, g, b = 0, c, x
	case h < 4.0/6:
		r, g, b = 0, x, c
	case h < 5.0/6:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.NRGBA{
		uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255), 255,
	}
}
