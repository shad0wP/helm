package icon

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestGenerateIconProducesValid22pxPNG(t *testing.T) {
	colours := map[string]color.RGBA{
		"green": ColorGreen,
		"amber": ColorAmber,
		"gray":  ColorGray,
	}
	for name, c := range colours {
		t.Run(name, func(t *testing.T) {
			raw := GenerateIcon(c)
			if len(raw) == 0 {
				t.Fatal("GenerateIcon returned zero bytes")
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("output is not a decodable PNG: %v", err)
			}
			b := img.Bounds()
			if b.Dx() != 22 || b.Dy() != 22 {
				t.Errorf("icon size = %dx%d, want 22x22", b.Dx(), b.Dy())
			}
		})
	}
}

func TestGenerateIconIsDeterministicAndColourDistinct(t *testing.T) {
	green := GenerateIcon(ColorGreen)
	amber := GenerateIcon(ColorAmber)
	gray := GenerateIcon(ColorGray)

	// Different state colours must produce visibly different icons.
	if bytes.Equal(green, amber) || bytes.Equal(green, gray) || bytes.Equal(amber, gray) {
		t.Error("expected a distinct icon per state colour")
	}
	// Same colour must be reproducible byte-for-byte.
	if !bytes.Equal(green, GenerateIcon(ColorGreen)) {
		t.Error("GenerateIcon is not deterministic for the same colour")
	}
}

func TestGenerateIconGeometry(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(GenerateIcon(ColorGreen)))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// The filled centre hub must be opaque and green-dominant (52,199,89).
	cr, cg, cb, ca := img.At(11, 11).RGBA()
	if ca == 0 {
		t.Fatal("centre pixel is fully transparent; expected an opaque hub")
	}
	r8, g8, b8 := uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)
	if !(g8 > r8 && g8 > b8) {
		t.Errorf("centre colour (%d,%d,%d) is not green-dominant", r8, g8, b8)
	}

	// The wheel never reaches the corners, so they must be transparent.
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Errorf("corner (0,0) alpha = %d, want 0 (transparent)", a)
	}
	if _, _, _, a := img.At(21, 21).RGBA(); a != 0 {
		t.Errorf("corner (21,21) alpha = %d, want 0 (transparent)", a)
	}
}

func TestIconGlobalsInitialised(t *testing.T) {
	globals := map[string][]byte{
		"IconGreen": IconGreen,
		"IconAmber": IconAmber,
		"IconGray":  IconGray,
	}
	for name, raw := range globals {
		if len(raw) == 0 {
			t.Errorf("%s was not initialised by init()", name)
			continue
		}
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			t.Errorf("%s is not a valid PNG: %v", name, err)
		}
	}
}
