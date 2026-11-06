package app

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var errCaptureCancelled = errors.New("截图已取消")

type screenSelection struct {
	Image  image.Image
	Bounds image.Rectangle
}

func captureRegion() (image.Image, error) {
	selection, err := selectScreen("拖动选择区域，松开自动复制 · Esc / 右键取消")
	return selection.Image, err
}

func rgbaFromBGRA(data []byte, stride int, rect image.Rectangle) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			src := (rect.Min.Y+y)*stride + (rect.Min.X+x)*4
			dst := y*img.Stride + x*4
			img.Pix[dst], img.Pix[dst+1], img.Pix[dst+2], img.Pix[dst+3] = data[src+2], data[src+1], data[src], 255
		}
	}
	return img
}

func selectionRect(start, end image.Point, bounds image.Rectangle) image.Rectangle {
	return image.Rect(min(start.X, end.X), min(start.Y, end.Y), max(start.X, end.X), max(start.Y, end.Y)).Intersect(bounds)
}

func (a *app) capture() {
	if a.capturing || a.quitting || a.scrollTask != nil || a.recordTask != nil {
		return
	}
	a.capturing = true
	wasVisible := a.window.IsVisible()
	a.window.Hide()
	go func() {
		// Let the compositor remove the control panel before taking the snapshot.
		time.Sleep(180 * time.Millisecond)
		img, err := captureRegion()
		var data bytes.Buffer
		if err == nil {
			err = png.Encode(&data, img)
		}
		mygo.RunOnMain(func() {
			defer a.window.Invalidate()
			a.capturing = false
			if a.quitting {
				return
			}
			if errors.Is(err, errCaptureCancelled) {
				a.status = "截图已取消"
				if wasVisible {
					a.window.Show()
				}
				return
			}
			if err != nil {
				a.status = "截图失败：" + err.Error()
				a.window.Show()
				return
			}
			a.publishImage(img, data.Bytes(), "截图_"+time.Now().Format("20060102_150405")+".png")
		})
	}()
}

func (a *app) publishImage(img image.Image, data []byte, name string) {
	a.bitmap = ui.NewBitmap(img).Pixelated()
	a.data = bytes.Clone(data)
	a.zoom = 1
	a.actualPixels = false
	a.pan = ui.Point{}
	a.name = name
	if err := a.writeClipboardImage(a.data); err != nil {
		a.status = "图片已保留，但复制失败：" + err.Error()
		a.page = "图片"
		a.window.Show()
		return
	}
	a.status = fmt.Sprintf("已复制 %d × %d 原始像素 PNG，可直接粘贴", img.Bounds().Dx(), img.Bounds().Dy())
	if key := a.activeHotkeys["pin"]; key != "" {
		a.status += " · " + displayHotkey(key) + " 贴到桌面"
	}
	if a.tray != nil {
		a.tray.SetToolTip("映刻 · " + a.status)
	}
	if a.autoPin {
		status := a.status
		a.pinAsset(a.bitmap, a.data, a.name, false)
		a.status = status
	}
}
