package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestImagePinHasRoundedCornersAndPinToggle(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 240, 160))
	ink := color.RGBA{24, 84, 137, 255}
	for y := 0; y < 160; y++ {
		for x := 0; x < 240; x++ {
			img.SetRGBA(x, y, ink)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	s := &pinState{app: newApp(), bitmap: ui.NewBitmap(img), data: bytes.Clone(encoded.Bytes())}
	tt := ui.NewTester(s.view, 240, 160)
	tt.Frame()
	for _, label := range []string{"常驻", "1:1", "关闭", "隐藏", "临时贴图 · 点击窗口外关闭", "已常驻 · Esc 关闭"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("unwanted pin chrome: %s", label)
		}
	}
	for _, point := range []image.Point{{0, 0}, {239, 0}, {0, 159}, {239, 159}} {
		if tt.Image().RGBAAt(point.X, point.Y).A != 0 {
			t.Fatalf("corner is not transparent: %v", point)
		}
	}
	for _, point := range []image.Point{{100, 0}, {100, 159}, {0, 80}, {239, 80}, {100, 80}} {
		if got := tt.Image().RGBAAt(point.X, point.Y); got != ink {
			t.Fatalf("image does not reach window edge at %v: %+v", point, got)
		}
	}
	corner := image.Pt(222, 18)
	if tt.Image().RGBAAt(corner.X, corner.Y) != ink {
		t.Fatal("the idle pin obscures the image")
	}
	tt.Move(float32(corner.X), float32(corner.Y))
	tt.Frame()
	if tt.Image().RGBAAt(corner.X, corner.Y) == ink {
		t.Fatal("the pin did not appear on hover")
	}
	tt.Move(110, 75)
	tt.Frame()
	if tt.Image().RGBAAt(corner.X, corner.Y) != ink {
		t.Fatal("the pin remained visible after leaving the corner")
	}
	if err := tt.Click("固定贴图"); err != nil {
		t.Fatal(err)
	}
	if !s.resident {
		t.Fatal("pin button did not fix image")
	}
	if err := tt.Click("取消固定贴图"); err != nil {
		t.Fatal(err)
	}
	if s.resident || !bytes.Equal(s.data, encoded.Bytes()) {
		t.Fatal("pin toggle changed image data or did not unfix image")
	}
}

func TestSmallPinUsesContextMenu(t *testing.T) {
	s := &pinState{app: newApp(), bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 160, 100)))}
	tt := ui.NewTester(s.view, 160, 100)
	tt.Frame()
	if _, ok := tt.Find("固定贴图"); ok {
		t.Fatal("small image has a pin button")
	}
	if err := tt.RightClick("贴图图片"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("保持显示"); err != nil {
		t.Fatal(err)
	}
	if !s.resident {
		t.Fatal("context menu did not fix the small image")
	}
}

func TestPinDimensionsFitImageAtDisplayDensity(t *testing.T) {
	for _, test := range []struct {
		width, height int
		density       float64
		wantW, wantH  int
	}{{160, 200, 1.25, 128, 160}, {160, 200, 2, 80, 100}, {3200, 1800, 1.25, 900, 506}} {
		w, h := pinDimensions(test.width, test.height, test.density, 900, 720)
		if w != test.wantW || h != test.wantH {
			t.Fatalf("wrong image window size: %dx%d, want %dx%d", w, h, test.wantW, test.wantH)
		}
	}
}
