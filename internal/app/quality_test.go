package app

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCapturePNGPreservesEveryPhysicalPixel(t *testing.T) {
	const width, height = 257, 129
	stride := width*4 + 12
	data := make([]byte, stride*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := y*stride + x*4
			data[offset], data[offset+1], data[offset+2] = byte(x*17+y*3), byte(x*7+y*19), byte(x*31+y*11)
		}
	}
	rect := image.Rect(3, 4, 241, 101)
	original := rgbaFromBGRA(data, stride, rect)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, original); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Size() != rect.Size() {
		t.Fatal("PNG dimensions changed")
	}
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			r, g, b, a := decoded.At(x, y).RGBA()
			src := (rect.Min.Y+y)*stride + (rect.Min.X+x)*4
			if byte(r>>8) != data[src+2] || byte(g>>8) != data[src+1] || byte(b>>8) != data[src] || a != 65535 {
				t.Fatalf("pixel changed at %d,%d", x, y)
			}
		}
	}
}

func TestPinKeepsNativeScaleWhenDIPSizeRounds(t *testing.T) {
	s := pinState{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 257, 129)))}
	const density = 1.5
	if scale := s.imageScale(171, 86, density); scale != float32(1/density) {
		t.Fatalf("rounded window reduced the source to %v of its pixel size", scale*density)
	}
	if scale := s.imageScale(128, 64, density); scale >= float32(1/density) {
		t.Fatal("a genuinely smaller window must still fit the image")
	}
}
