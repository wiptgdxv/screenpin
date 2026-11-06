package app

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func trayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{37, 99, 235, 255}), image.Point{}, draw.Src)
	for _, r := range []image.Rectangle{image.Rect(7, 6, 10, 25), image.Rect(6, 7, 25, 10), image.Rect(22, 7, 25, 26), image.Rect(7, 22, 26, 25)} {
		draw.Draw(img, r, image.NewUniform(color.White), image.Point{}, draw.Src)
	}
	var data bytes.Buffer
	_ = png.Encode(&data, img)
	return data.Bytes()
}

func (a *app) showPanel() { a.page = "工作台"; a.window.Show() }
func (a *app) hidePanel() {
	if a.tray == nil {
		a.status = "托盘不可用，窗口保持打开以便操作"
		return
	}
	a.window.Hide()
}

func (a *app) setupBackground() {
	click := func(action func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { action() }
	}
	a.latestVideoMenu = &mygo.MenuItem{Label: "打开最近录屏", Disabled: a.lastVideo == "", Click: click(func() { a.openRecording(a.lastVideo) })}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "区域截图", Click: click(a.capture)},
		{Label: "滚动截图 / 完成长图", Click: click(a.scrollCapture)},
		{Label: "录屏 / 停止录屏", Click: click(a.recordVideo)},
		{Label: "剪贴板贴图", Click: click(func() { a.pasteImage(true) })},
		{Type: mygo.MenuItemSeparator},
		{Label: "显示面板", Click: click(a.showPanel)},
		{Label: "设置", Click: click(func() { a.page = "设置"; a.window.Show() })},
		{Label: "打开录屏目录", Click: click(a.openVideoDirectory)},
		a.latestVideoMenu,
		{Type: mygo.MenuItemSeparator},
		{Label: "退出映刻", Click: click(mygo.App.Quit)},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "screenpin-main", Icon: trayIcon(), ToolTip: "映刻 Screenpin · 快捷键截图与贴图", Menu: menu})
	if err != nil {
		a.status = "托盘启动失败：" + err.Error()
		return
	}
	a.tray = tray
	a.trayMenu = menu
	tray.OnClick(a.showPanel)
	a.window.OnClose(func(event *mygo.CloseEvent) {
		if !a.quitting {
			event.PreventDefault()
			a.hidePanel()
		}
	})
	mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
		if a.recordTask != nil {
			event.PreventDefault()
			a.quitAfterPreview = true
			a.stopRecording(true)
			return
		}
		if a.recordPreview != nil {
			event.PreventDefault()
			a.quitAfterPreview = true
			a.recordPreview.present()
			a.recordPreview.window.Close()
			return
		}
		a.quitting = true
		cancelCapture()
		if a.scrollTask != nil {
			a.scrollTask.finish(false)
		}
	})
	mygo.App.OnQuit(tray.Destroy)
	if a.startHidden {
		a.window.Hide()
	}
}

func (a *app) startPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(16, 24).Gap(8).Children(func() {
				launch := func(id, icon, title, detail, label string, run func()) {
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Padding(4, 10).Children(func() {
						ui.Icon(c, icons[icon]).Size(18, 18).TextColor(t.Accent).Shrink(0)
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							ui.Text(c, title).FontSize(14).Bold()
							ui.Text(c, detail).FontSize(12).TextColor(t.TextMuted)
						})
						key := displayHotkey(a.activeHotkeys[id])
						if key == "" {
							key = "未启用"
						}
						ui.Text(c, key).FontSize(12).TextColor(t.TextMuted).Padding(4, 8).Background(t.Surface).Border(1, t.Border).Radius(5).Shrink(0)
						if a.button(c, "launch-"+id, "", label, id == "capture").Width(94).Clicked() {
							run()
						}
					})
				}
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
						ui.Text(c, "截图工作台").FontSize(23).Bold()
						ui.Text(c, "全局快捷键在面板收起后也能使用。").FontSize(12).TextColor(t.TextMuted)
					})
					if a.action(c, "workbench-open", "folder", "打开图片", false) {
						a.open()
					}
				})
				rule(c)
				launch("capture", "fit", "区域截图", "框选后自动复制，直接粘贴到其他应用。", "开始截图", a.capture)
				launch("pin", "tack", "剪贴板贴图", "先复制图片，再把它贴成桌面浮窗。", "贴图", func() { a.pasteImage(true) })
				if a.hasImage() {
					rule(c)
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Padding(4, 10).Children(func() {
						ui.Text(c, "当前图片 · "+a.name).SingleLine().Grow(1).MinWidth(0).FontSize(12).TextColor(t.TextMuted)
						if a.action(c, "view-current", "", "查看图片", false) {
							a.page = "图片"
						}
						if a.action(c, "ocr-current", "", "提取文字", false) {
							a.recognizeImage(a.data, false)
						}
						if a.action(c, "qr-current", "", "识别二维码", false) {
							a.recognizeImage(a.data, true)
						}
						if a.action(c, "save-current", "save", "保存 PNG", false) {
							a.save()
						}
					})
				}
				rule(c)
				scrollLabel, recordLabel := "开始滚动", "开始录屏"
				if a.scrollTask != nil {
					scrollLabel = "完成长图"
				}
				if a.recordTask != nil {
					recordLabel = "结束录制"
				}
				launch("scroll", "long-capture", "滚动长图", "框选内容后滚动，自动识别方向并拼接。", scrollLabel, a.scrollCapture)
				launch("record", "video", "区域录屏", "框选后立即录制，结束后可预览和导出。", recordLabel, a.recordVideo)
				ui.Row(c).FillWidth().Padding(0, 10, 3, 40).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "录屏声音").FontSize(12).TextColor(t.TextMuted).Grow(1)
					ui.Text(c, "系统声音").FontSize(12)
					ui.Switch(c, &a.recordSystem).Label("系统声音").Disabled(a.recordTask != nil)
					ui.Box(c).Width(8)
					ui.Text(c, "麦克风").FontSize(12)
					ui.Switch(c, &a.recordMic).Label("录麦克风").Disabled(a.recordTask != nil)
				})
				if a.lastVideo != "" {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, "最近录屏  "+filepath.Base(a.lastVideo)).SingleLine().Grow(1).MinWidth(0).FontSize(12).TextColor(t.TextMuted)
						if a.action(c, "play-recording", "", "打开视频", false) {
							a.openRecording(a.lastVideo)
						}
						if a.action(c, "video-directory", "folder", "定位文件", false) {
							mygo.Shell.ShowItemInFolder(a.lastVideo)
						}
					})
				}
			})
		})
		ui.Row(c).FillWidth().Height(46).Shrink(0).Padding(0, 20).Gap(8).AlignItems(ui.Center).Children(func() {
			hint := "关闭窗口将收起到托盘。"
			if key := displayHotkey(a.activeHotkeys["show"]); key != "" {
				hint += "按 " + key + " 唤回面板。"
			}
			ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted).Grow(1)
			if a.action(c, "hide-panel", "", "收起到托盘", false) {
				a.hidePanel()
			}
			if a.action(c, "quit-app", "", "退出程序", false) {
				mygo.App.Quit()
			}
		})
	})
}
