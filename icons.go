package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Tray icon state colours (Apple semantic palette).
var (
	ColorGreen = color.RGBA{R: 52, G: 199, B: 89, A: 255}   // #34C759
	ColorAmber = color.RGBA{R: 255, G: 159, B: 10, A: 255}  // #FF9F0A
	ColorGray  = color.RGBA{R: 142, G: 142, B: 147, A: 255} // #8E8E93
)

// Pre-rendered PNG bytes for each state, built once at init.
var (
	IconGreen []byte
	IconAmber []byte
	IconGray  []byte
)

func init() {
	IconGreen = GenerateIcon(ColorGreen)
	IconAmber = GenerateIcon(ColorAmber)
	IconGray = GenerateIcon(ColorGray)
}

// GenerateIcon renders a 22×22 nautical helm-wheel glyph in the given colour
// and returns the encoded PNG bytes. The shape is a rim ring, eight spokes
// (drawn as four diameters) that extend slightly past the rim as handles, and a
// filled central hub. The image is supersampled and box-filtered for clean,
// anti-aliased edges with straight (non-premultiplied) alpha.
func GenerateIcon(c color.RGBA) []byte {
	const (
		out  = 22       // final icon size (macOS menu-bar standard)
		ss   = 4        // supersampling factor
		size = out * ss // working resolution
	)

	cx, cy := float64(size)/2, float64(size)/2
	rimOuter := float64(size) * 0.40
	rimInner := float64(size) * 0.32
	hubR := float64(size) * 0.12
	spokeHalf := float64(size) * 0.040
	spokeOuter := float64(size) * 0.47 // handles poke past the rim

	covered := make([]bool, size*size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			dist := math.Hypot(dx, dy)

			on := false
			switch {
			case dist <= hubR: // filled centre hub
				on = true
			case dist >= rimInner && dist <= rimOuter: // rim ring
				on = true
			case dist <= spokeOuter: // spokes / handles
				for k := 0; k < 4; k++ { // four diameters → eight spokes
					a := float64(k) * math.Pi / 4
					if math.Abs(dx*math.Sin(a)-dy*math.Cos(a)) <= spokeHalf {
						on = true
						break
					}
				}
			}
			if on {
				covered[y*size+x] = true
			}
		}
	}

	// Downsample with a box filter; alpha = covered-sample fraction, RGB held at
	// the source colour so edges fade cleanly to transparent.
	dst := image.NewNRGBA(image.Rect(0, 0, out, out))
	const samples = ss * ss
	for y := 0; y < out; y++ {
		for x := 0; x < out; x++ {
			cnt := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					if covered[(y*ss+sy)*size+(x*ss+sx)] {
						cnt++
					}
				}
			}
			a := uint8(cnt * 255 / samples)
			dst.SetNRGBA(x, y, color.NRGBA{R: c.R, G: c.G, B: c.B, A: a})
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, dst)
	return buf.Bytes()
}
