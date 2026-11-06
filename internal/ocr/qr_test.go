package ocr

import (
	"image"
	"image/color"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func TestDecodeQRPreservesPayload(t *testing.T) {
	const payload = "https://example.com/?value=1%2B2"
	matrix, err := qrcode.NewQRCodeWriter().Encode(payload, gozxing.BarcodeFormat_QR_CODE, 240, 240, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if !matrix.Get(x, y) {
				img.SetGray(x, y, color.Gray{255})
			}
		}
	}
	if got, err := DecodeQR(img); err != nil || got != payload {
		t.Fatalf("%q, %v", got, err)
	}
}
