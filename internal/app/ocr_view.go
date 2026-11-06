package app

import (
	"bytes"
	"image/png"

	"screenpin/internal/ocr"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *app) recognizeImage(data []byte, qr bool) {
	if len(data) == 0 {
		return
	}
	title := "提取文字"
	if qr {
		title = "二维码识别"
	}
	content, status, ready := "", "正在识别…", false
	engine, language, pending := "自动", "自动", true
	lastEngine, lastLanguage := engine, language
	var recognize func()
	var window *mygo.Window
	material := a.backdropMaterial()
	window = mygo.NewWindow(mygo.WindowOptions{Title: title, Width: 560, Height: 390, MinWidth: 420, MinHeight: 280, AlwaysOnTop: true, Transparent: a.clearBackground(), Vibrancy: material, Content: ui.View(func(c *ui.Context) {
		a.setTheme(c)
		c.Root().Background(a.windowBackground(c))
		ui.Column(c).Fill().Padding(18).Gap(12).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, title).Bold().FontSize(17).Grow(1)
				if !qr {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, "引擎").FontSize(11).TextColor(c.Theme().TextMuted)
						ui.Select(c.Key("ocr-engine"), &engine, []string{"自动", "微信 OCR", "Windows OCR"}).Label("识别引擎").Width(132).MinWidth(132).Disabled(pending)
					})
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, "语言").FontSize(11).TextColor(c.Theme().TextMuted)
						selection := &language
						automatic := "自动"
						if engine == "微信 OCR" {
							selection = &automatic
						}
						ui.Select(c.Key("ocr-language"), selection, []string{"自动", "简体中文", "英语"}).Label("识别语言").Width(110).MinWidth(110).Disabled(pending || engine == "微信 OCR").Tooltip("微信 OCR 自动识别语言")
					})
					if !pending && (engine != lastEngine || language != lastLanguage) {
						recognize()
					}
				}
			})
			if ready {
				ui.TextArea(c.Key("recognized-text"), &content).Grow(1).MinHeight(0).FillWidth().Padding(12).ReadOnly(true).Label("识别结果")
			} else {
				ui.Column(c).Grow(1).MinHeight(0).FillWidth().Padding(16).Gap(8).Justify(ui.Center).Background(c.Theme().Surface).Radius(6).Children(func() {
					ui.Text(c, status).FontSize(14)
					if !pending && !qr {
						ui.Text(c, "可切换引擎或语言，也可重新截取字迹清晰的区域。").FontSize(12).TextColor(c.Theme().TextMuted)
					}
				})
			}
			ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
				if ready {
					ui.Text(c, status).Grow(1).MinWidth(0).FontSize(12).TextColor(c.Theme().TextMuted)
				} else {
					ui.Box(c).Grow(1)
				}
				if !qr && !pending && !ready && a.action(c, "retry-recognized", "", "重新识别", false) {
					recognize()
				}
				if a.button(c, "copy-recognized", "", "复制全部", true).Disabled(!ready).Clicked() {
					if err := mygo.Clipboard.Write(transfer.TextData(content)); err != nil {
						status = "复制失败，请重试"
					} else {
						status = "已复制，可直接粘贴"
					}
				}
			})
		})
	})})
	closed := false
	window.OnClosed(func() { closed = true })
	window.OnBlur(func() { window.SetAlwaysOnTop(false) })
	recognize = func() {
		lastEngine, lastLanguage = engine, language
		pending, ready, content, status = true, false, "", "正在识别…"
		selectedEngine := map[string]string{"微信 OCR": ocr.WeChat, "Windows OCR": ocr.Windows}[engine]
		tag := map[string]string{"简体中文": "zh-Hans-CN", "英语": "en-US"}[language]
		go func() {
			img, err := png.Decode(bytes.NewReader(data))
			var value, engine string
			if err == nil {
				if qr {
					value, err = ocr.DecodeQR(img)
				} else {
					value, engine, err = ocr.RecognizeText(img, selectedEngine, tag)
				}
			}
			mygo.RunOnMain(func() {
				if closed {
					return
				}
				pending = false
				if err != nil {
					status = err.Error()
				} else if value == "" {
					status = "这张图片里没有识别到内容"
				} else {
					content = value
					ready = true
					status = "可选中部分内容，或复制全部"
					if engine != "" {
						status = engine + " · 可选中或复制文字"
					}
				}
				window.Invalidate()
			})
		}()
	}
	recognize()
	window.Show()
}
