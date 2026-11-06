package app

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestFrostedAppearanceCoversWholeWindow(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, page := range []string{"图片", "设置", "工作台"} {
			t.Run(fmt.Sprintf("dark-%v/%s", dark, page), func(t *testing.T) {
				a := newTestApp()
				a.dark = dark
				a.page = page
				original := bytes.Clone(a.data)
				a.glassTransparency = 20
				tt := ui.NewTester(a.view, 900, 600)
				tt.SetVibrancy(true)
				tt.Frame()
				points := []image.Point{{500, 12}, {500, 64}, {8, 250}, {500, 590}}
				if page == "图片" {
					points = append(points, image.Pt(500, 550))
				}
				for _, point := range points {
					if alpha := tt.Image().RGBAAt(point.X, point.Y).A; alpha == 255 {
						t.Fatalf("opaque region at %v hides backdrop", point)
					}
				}
				if page == "图片" && tt.Image().RGBAAt(450, 300).A != 255 {
					t.Fatal("glass made image content translucent")
				}
				a.frosted = false
				a.glassTransparency = 0
				tt.Frame()
				for _, point := range points {
					if tt.Image().RGBAAt(point.X, point.Y).A != 255 {
						t.Fatalf("opaque mode left transparency at %v", point)
					}
				}
				a.frosted = true
				tt.SetVibrancy(false)
				tt.Frame()
				for _, point := range points {
					if tt.Image().RGBAAt(point.X, point.Y).A != 255 {
						t.Fatalf("unsupported system left transparency at %v", point)
					}
				}
				if !bytes.Equal(original, a.data) {
					t.Fatal("appearance changed image bytes")
				}
			})
		}
	}
}
