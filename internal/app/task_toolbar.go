package app

import (
	"image"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) showTaskWindow(title string, bounds image.Rectangle, finish, cancel func()) {
	recording := title == "区域录屏"
	a.closeTaskOutline = showRegionOutline(bounds, recording)
	options := mygo.WindowOptions{
		Title: title, Width: 322, Height: 42, MinWidth: 322, MinHeight: 42,
		AlwaysOnTop: true, SkipTaskbar: true, DisableShadow: true, DisableResize: true,
		DisableMove: true, DisableMaximize: true, DisableMinimize: true,
		Frameless: true, HideWindowButtons: true, CornerRadius: 9, Vibrancy: mygo.VibrancyAcrylic,
		Content: ui.View(func(c *ui.Context) { a.taskToolbar(c, recording, finish, cancel) }),
	}
	window := mygo.NewWindow(options)
	hideNativeWindowBorder(window.NativeHandle())
	window.SetVibrancy(a.backdropMaterial())
	a.taskWindow = window
	window.OnClose(func(*mygo.CloseEvent) { cancel() })
	if !placeTaskWindow(window.NativeHandle(), bounds, recording) {
		window.Hide()
	} else {
		window.ShowInactive()
	}
}

func (a *app) taskToolbar(c *ui.Context, recording bool, finish, cancel func()) {
	a.setTheme(c)
	background := ui.Hex("#1A252C")
	if a.clearBackground() {
		background = background.Alpha(.65 + .35*a.clearTint())
	} else if a.frosted && c.Vibrancy() {
		background = background.Mix(ui.Hex("#2B414C"), a.glassDensityFraction()).Alpha(a.glassTint(.62))
	}
	c.Root().Background(background)
	light, muted := ui.Hex("#ECF3F6"), ui.Hex("#B8CBD2")
	accent := ui.Hex("#7ABDEB")
	if recording {
		accent = ui.Hex("#F4858C")
	}
	finishLabel, finishTooltip, finishText, cancelLabel, cancelTooltip := "完成", "完成并复制长截图", "完成", "取消滚动截图", "取消并丢弃长截图"
	if recording {
		finishLabel, finishTooltip, finishText, cancelLabel, cancelTooltip = "结束录制", "结束录制并打开预览", "结束", "取消录制", "取消并丢弃录制"
	}
	toolbarButton := func(key string) ui.Element {
		button := ui.ButtonBase(c.Key(key)).Height(28).Padding(0, 6).Radius(5).TextColor(light)
		if button.Hovered() {
			button.Background(ui.Hex("#39474F"))
		}
		return button
	}
	ui.Row(c).Fill().Padding(0, 8, 0, 11).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(accent)
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			if elapsed, running := strings.CutPrefix(a.taskMessage, "录屏中  "); running {
				ui.Text(c, "录屏中").FontSize(11).TextColor(muted)
				ui.Text(c, elapsed).FontSize(14).Bold().TextColor(light)
			} else {
				ui.Text(c, a.taskMessage).FontSize(12).SingleLine().TextColor(light).Tooltip(a.taskMessage)
			}
		})
		ui.Box(c).Size(1, 16).Background(ui.Hex("#40535C"))
		if s := a.recordTask; recording && s != nil && !s.finishing {
			label := "暂停"
			if s.paused {
				label = "继续"
			}
			ready := s.paused || (!s.pausing && s.segment != nil && s.segment.frames.Load() > 0)
			pause := toolbarButton("task-pause").Label(label + "录制").Disabled(!ready)
			pause.Children(func() { ui.Text(c, label).FontSize(12) })
			if pause.Clicked() && ready {
				a.toggleRecordingPause()
			}
		}
		if s := a.scrollTask; !recording && s != nil {
			undo := toolbarButton("scroll-undo").Label("撤回").Tooltip("撤回最后一段拼接")
			undo.Children(func() { ui.Text(c, "撤回").FontSize(12) })
			if undo.Clicked() {
				select {
				case s.undo <- struct{}{}:
				default:
				}
			}
		}
		stop := toolbarButton("task-finish").Padding(0, 8).Label(finishLabel).Tooltip(finishTooltip)
		stop.Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				if recording {
					ui.Icon(c, icons["stop-recording"]).Size(14, 14).TextColor(accent)
				}
				ui.Text(c, finishText).FontSize(12)
			})
		})
		if stop.Clicked() {
			finish()
		}
		closeButton := toolbarButton("task-cancel").Size(26, 28).Padding(0).Label(cancelLabel).Tooltip(cancelTooltip).TextColor(muted)
		closeButton.Children(func() {
			ui.Box(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, icons["cancel-recording"]).Size(14, 14)
			})
		})
		if closeButton.Clicked() {
			cancel()
		}
	})
}

func (a *app) refreshTaskWindow() {
	if a.taskWindow != nil {
		a.taskWindow.Invalidate()
	}
	if a.tray != nil {
		a.tray.SetToolTip("映刻 · " + a.taskMessage)
	}
}
func (a *app) closeTaskWindow() {
	if a.closeTaskOutline != nil {
		a.closeTaskOutline()
		a.closeTaskOutline = nil
	}
	if a.taskWindow != nil {
		window := a.taskWindow
		a.taskWindow = nil
		window.Destroy()
	}
}
