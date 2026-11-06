package app

import (
	"image"
	"testing"
)

func TestSelectionRectNormalizesAndClips(t *testing.T) {
	bounds := image.Rect(-1920, -400, 2560, 1440)
	for _, test := range []struct {
		name       string
		start, end image.Point
		want       image.Rectangle
	}{
		{"reverse", image.Pt(300, 200), image.Pt(100, 50), image.Rect(100, 50, 300, 200)},
		{"negative monitor", image.Pt(-1800, -300), image.Pt(-1600, -100), image.Rect(-1800, -300, -1600, -100)},
		{"clip virtual desktop", image.Pt(-3000, -900), image.Pt(4000, 2000), bounds},
		{"single click", image.Pt(10, 10), image.Pt(10, 10), image.Rectangle{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := selectionRect(test.start, test.end, bounds)
			if got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}
