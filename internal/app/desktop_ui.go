package app

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
)

func (a *app) hasImage() bool { return a.bitmap != nil && len(a.data) != 0 }

func rule(c *ui.Context) { ui.Box(c).Height(1).FillWidth().Shrink(0).Background(c.Theme().Border) }

func (a *app) desktop(c *ui.Context, t *ui.Theme, title ui.TitleBar, height float32) {
	c.Root().Background(a.windowBackground(c))
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(max(38, title.Height)).Shrink(0).Padding(0, title.Right+12, 0, title.Left+16).DragWindow().AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, icons["image"]).Size(16, 16).TextColor(t.Accent)
			ui.Text(c, "映刻 Screenpin").FontSize(12).Bold()
			ui.Box(c).Grow(1)
			appearance := a.button(c, "theme", "sun", "外观", false)
			if appearance.Clicked() {
				a.appearance = !a.appearance
			}
			a.appearanceMenu(c, appearance)
		})
		rule(c)
		ui.Row(c).Height(46).Shrink(0).Padding(0, 16).Gap(6).AlignItems(ui.Center).Background(t.Surface).Children(func() {
			for _, page := range []string{"工作台", "设置"} {
				button := a.button(c, "nav-"+page, "", page, false)
				if a.page == page || (page == "工作台" && a.page == "图片") {
					button.Background(t.SurfaceHover).TextColor(t.Accent)
				}
				if button.Clicked() {
					a.page = page
				}
			}
			ui.Box(c).Grow(1)
			if a.page == "图片" {
				if a.action(c, "open", "folder", "打开图片", false) {
					a.open()
				}
				if a.action(c, "paste", "copy", "粘贴图片", false) {
					a.pasteImage(false)
				}
			}
		})
		rule(c)
		if a.page == "设置" {
			a.settings(c)
		} else if a.page == "工作台" {
			a.startPage(c)
		} else {
			a.workspace(c, false, height < 620)
		}
		rule(c)
		ui.Row(c).Height(26).Shrink(0).Padding(0, 16).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.status).SingleLine().Grow(1).MinWidth(0).FontSize(11).TextColor(t.TextMuted)
			if ui.ButtonBase(c.Key("shortcut-help")).Height(24).Padding(0, 4).Label("快捷键").Children(func() { ui.Text(c, "快捷键  F1").FontSize(11).TextColor(t.TextMuted) }).Clicked() {
				a.help = true
			}
		})
	})
}

func (a *app) workspace(c *ui.Context, _ bool, short bool) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if !a.hasImage() {
			ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Icon(c, icons["image"]).Size(36, 36).TextColor(t.TextMuted)
				ui.Text(c, "打开图片，开始处理").FontSize(19).Bold()
				ui.Text(c, "也可以从剪贴板粘贴，保留图片原始尺寸。").FontSize(13).TextColor(t.TextMuted)
				if a.action(c, "empty-paste", "", "从剪贴板粘贴", true) {
					a.pasteImage(false)
				}
				ui.Text(c, displayHotkey("CmdOrCtrl+O")+" 打开    /    "+displayHotkey("CmdOrCtrl+V")+" 粘贴").FontSize(12).TextColor(t.TextMuted)
			})
		} else {
			ui.Row(c).Height(38).Shrink(0).Padding(0, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.name).SingleLine().Grow(1).MinWidth(0).FontSize(12)
				info := a.button(c, "image-info", "", "图像信息", false)
				if info.Clicked() {
					a.information = !a.information
				}
				ui.PopoverBase(c, info, &a.information, func(panel ui.Element) {
					panel.Width(248).Padding(16).Radius(6)
					a.material(panel, t)
					panel.Children(func() {
						ui.Column(c).Gap(10).Children(func() {
							ui.Text(c, "图像信息").Bold()
							width, height := a.bitmap.Size()
							ui.Textf(c, "%d × %d 像素", width, height)
							ui.Text(c, "PNG · 原尺寸导出").FontSize(12).TextColor(t.TextMuted)
							ui.Text(c, "缩放仅影响预览，保存尺寸不变。").FontSize(12).TextColor(t.TextMuted)
						})
					})
				})
			})
			canvas := a.canvasBackground(c)
			preview := ui.ButtonBase(c.Key("image-canvas")).Label("图片预览，拖动平移").Grow(1).MinHeight(0).FillWidth().Clip().Background(canvas)
			if dx, dy, dragging := preview.Dragged(); dragging {
				a.pan.X += dx
				a.pan.Y += dy
			}
			preview.Draw(func(p *ui.Painter, r ui.Rect) {
				bw, bh := a.bitmap.Size()
				pad := float32(24)
				if short {
					pad = 16
				}
				fitScale := min(max(1, r.W-2*pad)/float32(bw), max(1, r.H-2*pad)/float32(bh), 1/p.Scale())
				scale := fitScale * a.zoom
				if a.actualPixels {
					scale = a.zoom / p.Scale()
				}
				box := ui.Rect{X: r.X + (r.W-float32(bw)*scale)/2, Y: r.Y + (r.H-float32(bh)*scale)/2, W: float32(bw) * scale, H: float32(bh) * scale}
				limitX, limitY := max(0, (box.W-r.W)/2), max(0, (box.H-r.H)/2)
				a.pan.X = max(-limitX, min(limitX, a.pan.X))
				a.pan.Y = max(-limitY, min(limitY, a.pan.Y))
				box.X += a.pan.X
				box.Y += a.pan.Y
				box.X = float32(math.Round(float64(box.X*p.Scale()))) / p.Scale()
				box.Y = float32(math.Round(float64(box.Y*p.Scale()))) / p.Scale()
				p.Clip(r, 0, func() {
					p.Clip(box, 0, func() {
						p.Fill(box, ui.Hex("#FFFFFF"), 0)
						// Transparency is indicated only within the image, not across the canvas.
						visible := ui.Rect{X: max(r.X, box.X), Y: max(r.Y, box.Y), W: min(r.X+r.W, box.X+box.W) - max(r.X, box.X), H: min(r.Y+r.H, box.Y+box.H) - max(r.Y, box.Y)}
						for y := visible.Y; y < visible.Y+visible.H; y += 12 {
							for x := visible.X; x < visible.X+visible.W; x += 12 {
								if (int((x-visible.X)/12)+int((y-visible.Y)/12))%2 == 0 {
									p.Fill(ui.Rect{X: x, Y: y, W: 12, H: 12}, ui.Hex("#F0F0F0"), 0)
								}
							}
						}
						p.Image(a.bitmap, box, ui.Contain)
					})
				})
			})
		}
		rule(c)
		a.imageToolbar(c)
	})
}

func (a *app) imageToolbar(c *ui.Context) {
	t := c.Theme()
	empty := !a.hasImage()
	ui.Row(c).FillWidth().Height(44).Shrink(0).Padding(0, 12).Gap(2).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		if a.button(c, "zoom-out", "minus", "", false).Disabled(empty).Clicked() {
			a.zoom = max(.3, a.zoom-.15)
		}
		label := "—"
		if !empty {
			label = fmt.Sprintf("%.0f%%", a.zoom*100)
			if !a.actualPixels && a.zoom == 1 {
				label = "适应"
			}
		}
		ui.Text(c, label).Width(44).TextAlign(ui.Center).FontSize(12).TextColor(t.TextMuted)
		if a.button(c, "zoom-in", "plus", "", false).Disabled(empty).Clicked() {
			a.zoom = min(3, a.zoom+.15)
		}
		if a.button(c, "zoom-fit", "fit", "适应", false).Disabled(empty).Clicked() {
			a.zoom = 1
			a.actualPixels = false
			a.pan = ui.Point{}
		}
		if a.button(c, "actual-pixels", "", "1:1", a.actualPixels).Label("原始像素").Tooltip("按原始像素显示，保存始终使用原始尺寸").Disabled(empty).Clicked() {
			a.actualPixels = true
			a.zoom = 1
			a.pan = ui.Point{}
		}
		ui.Box(c).Grow(1)
		if a.button(c, "ocr-image", "", "提取文字", false).Disabled(empty).Clicked() {
			a.recognizeImage(a.data, false)
		}
		if a.button(c, "copy", "copy", "复制图片", false).Disabled(empty).Clicked() {
			a.copy()
		}
		if a.button(c, "save", "save", "保存 PNG", false).Disabled(empty).Clicked() {
			a.save()
		}
		if a.button(c, "pin", "pin", "贴到桌面", false).Disabled(empty).Clicked() {
			a.pin()
		}
	})
}

func (a *app) settings(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(18, 24, 12).Gap(10).Children(func() {
			ui.Text(c, "设置").FontSize(23).Bold()
			ui.Row(c).Gap(6).Children(func() {
				for index, label := range []string{"窗口外观", "常规", "快捷键"} {
					tab := a.button(c, "settings-"+label, "", label, false).Height(34)
					if a.settingsSection == index {
						tab.Background(t.SurfaceHover).TextColor(t.Accent)
					}
					if tab.Clicked() {
						a.settingsSection = index
					}
				}
			})
		})
		rule(c)
		ui.Scroll(c.Key(a.settingsSection)).Grow(1).MinHeight(0).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(16, 24).Gap(12).Children(func() {
				switch a.settingsSection {
				case 1:
					a.generalSettings(c)
				case 2:
					a.hotkeySettingsView(c)
				default:
					a.appearanceSettings(c)
				}
			})
		})
	})
}

func settingsSwitch(c *ui.Context, title, detail string, value *bool) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, title)
			ui.Text(c, detail).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		ui.Switch(c.Key(title), value).Label(title)
	})
}

func (a *app) appearanceSettings(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "显示效果").FontSize(16).Bold()
	settingsSwitch(c, "深色外观", "切换面板的颜色。", &a.dark)
	settingsSwitch(c, "毛玻璃效果", "在较低透明度下柔化背景。", &a.frosted)
	for _, setting := range []struct {
		key, title, detail string
		value              *float64
		maximum            float64
		disabled           bool
	}{
		{"glass-transparency", "背景透明度", "越往右，窗口背后的内容越明显。", &a.glassTransparency, maxGlassTransparency, !a.frosted && !clearCompositionSupported()},
		{"glass-density", "磨砂浓度", "越往右，透出的背景细节越淡。", &a.glassDensity, 100, !a.frosted},
	} {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, setting.title)
				ui.Text(c, setting.detail).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Slider(c.Key(setting.key), setting.value, 0, setting.maximum).Step(1).
				Width(180).Label(setting.title).Disabled(setting.disabled)
			ui.Textf(c, "%.0f%%", *setting.value).Width(40).TextAlign(ui.End).FontSize(12)
		})
	}
	rule(c)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(c, "窗口大小").Grow(1)
		for _, size := range []struct {
			name string
			w, h int
		}{{"紧凑", 760, 540}, {"标准", 960, 640}, {"宽屏", 1120, 720}} {
			if a.button(c, size.name, "", size.name, false).Clicked() && a.window != nil {
				a.window.SetSize(size.w, size.h)
			}
		}
	})
}

func (a *app) generalSettings(c *ui.Context) {
	ui.Text(c, "截图与贴图").FontSize(16).Bold()
	settingsSwitch(c, "截图后自动贴图", "截图会自动复制。开启后，还会临时贴到桌面。", &a.autoPin)
	rule(c)
	ui.Text(c, "启动与录屏").FontSize(16).Bold()
	hint := "再次启动程序可显示面板。"
	if key := displayHotkey(a.activeHotkeys["show"]); key != "" {
		hint = "按 " + key + " 或再次启动程序，可显示面板。"
	}
	settingsSwitch(c, "启动后收起到托盘", hint, &a.startHidden)
	rule(c)
	ui.Text(c, "录屏文件").FontSize(16).Bold()
	directory := a.recordingSaveDirectory()
	ui.Text(c, "默认保存目录").Bold()
	ui.Text(c, directory).FillWidth().SingleLine().Tooltip(directory).TextColor(c.Theme().TextMuted)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		if a.action(c, "choose-video-directory", "folder", "选择目录", true) {
			a.chooseVideoSaveDirectory()
		}
		if a.action(c, "settings-video-directory", "", "打开录屏目录", false) {
			a.openVideoDirectory()
		}
		if a.videoSaveDirectory != "" && a.action(c, "reset-video-directory", "", "恢复默认目录", false) {
			a.videoSaveDirectory = ""
			a.status = "已恢复默认录屏目录"
		}
	})
}
