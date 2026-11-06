package app

import (
	"bytes"
	"fmt"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

func newTestApp() *app {
	a := newApp()
	a.page = "图片"
	a.example()
	a.frosted = true
	a.nativeFrosted = true
	return a
}
func (a *app) example() {
	parsed, err := opentype.Parse(gomono.TTF)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 13, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	defer face.Close()
	img := image.NewRGBA(image.Rect(0, 0, 800, 500))
	fill := func(r image.Rectangle, c color.Color) {
		draw.Draw(img, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
	}
	fill(img.Bounds(), color.RGBA{248, 250, 253, 255})
	fill(image.Rect(0, 0, 800, 42), color.RGBA{232, 237, 245, 255})
	fill(image.Rect(0, 42, 162, 500), color.RGBA{240, 243, 248, 255})
	fill(image.Rect(181, 70, 776, 458), color.RGBA{255, 255, 255, 255})
	fill(image.Rect(198, 220, 754, 246), color.RGBA{229, 239, 255, 255})
	ink := color.RGBA{46, 60, 80, 255}
	blue := color.RGBA{36, 99, 235, 255}
	text := func(x, y int, s string, c color.Color) {
		d := font.Drawer{Dst: img, Src: &image.Uniform{C: c}, Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	text(22, 27, "workspace / api / tasks.go", ink)
	text(22, 85, "EXPLORER", ink)
	for i, s := range []string{"api/", "  tasks.go", "  routes.go", "store/", "  tasks.go", "go.mod", "README.md"} {
		text(22, 122+i*28, s, ink)
	}
	lines := []string{"package api", "", "func (s *Server) ListTasks(ctx context.Context) ([]Task, error) {", "    tasks, err := s.store.List(ctx)", "    if err != nil {", "        return nil, fmt.Errorf(\"list tasks: %w\", err)", "    }", "", "    return tasks, nil", "}", "", "// Capture a thought. Keep your flow."}
	for i, s := range lines {
		text(198, 103+i*27, fmt.Sprintf("%2d", i+1), color.RGBA{135, 148, 166, 255})
		text(232, 103+i*27, s, ink)
	}
	text(208, 433, "GET /v1/tasks      200 OK      28 ms", blue)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(err)
	}
	a.bitmap, a.data, a.name = ui.NewBitmap(img), b.Bytes(), "接口调试示例.png"
	a.status = "示例素材；可以打开自己的 PNG 图片"
	a.zoom = 1
}
