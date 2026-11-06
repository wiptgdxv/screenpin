package app

import (
	"bytes"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const pinCornerRadius = 14

type pinState struct {
	window       *mygo.Window
	app          *app
	bitmap       *ui.Bitmap
	data         []byte
	resident     bool
	actualPixels bool
	zoom         float64 // multiplier relative to the window fit
	density      float64
	zoomUntil    time.Time
	pan          ui.Point
}

// Sizes are DIPs; keep the image's aspect ratio and its physical pixels.
func pinDimensions(width, height int, density float64, availableWidth, availableHeight int) (int, int) {
	density = max(1, density)
	scale := min(1/density, min(float64(max(32, availableWidth))/float64(width), float64(max(32, availableHeight))/float64(height)))
	return max(32, int(math.Round(float64(width)*scale))), max(32, int(math.Round(float64(height)*scale)))
}

func (a *app) pinAsset(bitmap *ui.Bitmap, pngData []byte, name string, resident bool) {
	state := &pinState{app: a, bitmap: bitmap, data: bytes.Clone(pngData), resident: resident}
	imageWidth, imageHeight := bitmap.Size()
	display := mygo.Screen.PrimaryDisplay()
	width, height := pinDimensions(imageWidth, imageHeight, display.ScaleFactor, display.WorkArea.Width-48, display.WorkArea.Height-48)
	state.density = display.ScaleFactor
	state.window = mygo.NewWindow(mygo.WindowOptions{
		Title: "贴图 · " + name, Width: width, Height: height, MinWidth: 32, MinHeight: 32, UseContentSize: true,
		Frameless: true, HideWindowButtons: true, Transparent: true, CornerRadius: pinCornerRadius,
		DisableMinimize: true, DisableMaximize: true, DisableShadow: true,
		Panel: true, SkipTaskbar: true, AlwaysOnTop: true, Content: ui.View(state.view),
	})
	hideNativeWindowBorder(state.window.NativeHandle())
	if a.pins == nil {
		a.pins = map[*mygo.Window]*pinState{}
	}
	a.pins[state.window] = state
	state.window.OnBlur(func() {
		if !state.resident && !a.quitting {
			state.window.Close()
		}
	})
	state.window.OnResize(func() {
		state.density = mygo.Screen.DisplayMatching(state.window.Bounds()).ScaleFactor
		state.actualPixels = false
		state.zoomUntil = time.Now().Add(1250 * time.Millisecond)
		state.window.Invalidate()
	})
	state.window.OnClosed(func() { delete(a.pins, state.window) })
	a.status = "已贴到桌面：" + name
}

func (s *pinState) imageScale(viewWidth, viewHeight, density float32) float32 {
	fit := s.fitScale(viewWidth, viewHeight, density)
	if s.zoom > 0 {
		return fit * float32(s.zoom)
	}
	return fit
}

func (s *pinState) fitScale(viewWidth, viewHeight, density float32) float32 {
	width, height := s.bitmap.Size()
	// A window's size is rounded to whole DIPs. Keep native pixels aligned
	// when the requested 1:1 size differs only by that rounding.
	if math.Abs(float64(viewWidth-float32(width)/density)) <= 1 && math.Abs(float64(viewHeight-float32(height)/density)) <= 1 {
		return 1 / density
	}
	return min(viewWidth/float32(width), viewHeight/float32(height))
}

func (s *pinState) view(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Cmd, ui.KeyW) {
		if s.window != nil {
			s.window.Close()
		}
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) {
		if err := s.app.writeClipboardImage(s.data); err != nil {
			s.app.status = "贴图复制失败：" + err.Error()
		}
	}
	viewWidth, viewHeight := c.Size()
	density := float32(max(1, s.density))
	fitZoom := float64(s.fitScale(viewWidth, viewHeight, density) * density)
	if c.Shortcut(ui.Cmd, ui.Key0) && fitZoom > 0 {
		s.actualPixels = !s.actualPixels
		s.zoom = 0
		if s.actualPixels {
			s.zoom = 1 / fitZoom
		}
		s.pan = ui.Point{}
		s.zoomUntil = c.Now().Add(1250 * time.Millisecond)
	}
	s.app.setTheme(c)
	c.Root().Background(ui.Transparent)
	ui.Box(c).Fill().Radius(pinCornerRadius).Clip().HandleInput(func(event ui.InputEvent) bool {
		factor := float64(event.Scale)
		switch {
		case event.Kind == ui.InputScroll && event.DY != 0 && !event.Momentum:
			steps := math.Copysign(1, -float64(event.DY))
			if event.Precise {
				steps = -float64(event.DY) / 40
			}
			factor = math.Pow(1.15, steps)
		case event.Kind == ui.InputZoom && factor > 0:
		default:
			return false
		}
		oldZoom := float64(s.imageScale(viewWidth, viewHeight, density) * density)
		if oldZoom <= 0 {
			return false
		}
		newZoom := max(min(.10, fitZoom/4), min(max(8, fitZoom), oldZoom*factor))
		s.zoom = newZoom / fitZoom
		s.actualPixels = false
		ratio := float32(newZoom / oldZoom)
		x, y := event.X-viewWidth/2, event.Y-viewHeight/2
		s.pan = ui.Point{X: x - (x-s.pan.X)*ratio, Y: y - (y-s.pan.Y)*ratio}
		s.zoomUntil = event.Time.Add(1250 * time.Millisecond)
		return true
	}).Children(func() {
		preview := ui.Box(c.Key("pin-preview")).Fill().Radius(pinCornerRadius).Clip().Label("贴图图片")
		if s.actualPixels {
			if dx, dy, dragging := preview.Dragged(); dragging {
				s.pan.X += dx
				s.pan.Y += dy
			}
		} else {
			preview.DragWindow()
		}
		preview.Draw(func(p *ui.Painter, r ui.Rect) {
			width, height := s.bitmap.Size()
			scale := s.imageScale(r.W, r.H, p.Scale())
			w, h := float32(width)*scale, float32(height)*scale
			limitX, limitY := max(0, (w-r.W)/2), max(0, (h-r.H)/2)
			s.pan.X = max(-limitX, min(limitX, s.pan.X))
			s.pan.Y = max(-limitY, min(limitY, s.pan.Y))
			x, y := r.X+(r.W-w)/2+s.pan.X, r.Y+(r.H-h)/2+s.pan.Y
			x = float32(math.Round(float64(x*p.Scale()))) / p.Scale()
			y = float32(math.Round(float64(y*p.Scale()))) / p.Scale()
			p.Clip(r, pinCornerRadius, func() { p.Image(s.bitmap, ui.Rect{X: x, Y: y, W: w, H: h}, ui.Contain) })
		})
		preview.ContextMenu(func(menu *ui.Menu) {
			if menu.Item("保持显示").Checked(s.resident).Chosen() {
				s.resident = !s.resident
			}
			if menu.Item("提取文字").Chosen() {
				s.app.recognizeImage(s.data, false)
			}
			if menu.Item("识别二维码").Chosen() {
				s.app.recognizeImage(s.data, true)
			}
		})
		if remaining := s.zoomUntil.Sub(c.Now()); remaining > 0 {
			c.After(remaining)
			ui.Box(c).Absolute().Left(3).Bottom(3).Radius(6).Padding(2, 4).Background(ui.Hex("#161A1C").Alpha(.74)).Children(func() {
				ui.Textf(c, "%d%%", max(1, int(math.Round(float64(100*s.imageScale(viewWidth, viewHeight, density)*density))))).FontSize(10).TextColor(ui.Hex("#FFFFFF"))
			})
		}
		// Keep the control off narrow strips and small screenshots.
		if viewWidth < 240 || viewHeight < 160 {
			return
		}
		label, icon := "固定贴图", "tack"
		if s.resident {
			label, icon = "取消固定贴图", "tack-filled"
		}
		button := ui.ButtonBase(c.Key("pin-toggle")).Absolute().Top(3).Right(3).Size(30, 30).Radius(15).
			Label(label).Tooltip(label)
		if button.Hovered() || button.FocusVisible() {
			button.Children(func() {
				ui.Box(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					ui.Box(c).Size(22, 22).Radius(11).Background(ui.Hex("#161A1C").Alpha(.48)).
						TextColor(ui.Hex("#FFFFFF").Alpha(.94)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, icons[icon]).Size(13, 13)
					})
				})
			})
		}
		if button.Clicked() {
			s.resident = !s.resident
		}
	})
}
