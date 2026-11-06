package app

import (
	"bytes"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

var applicationName = "Screenpin"

type app struct {
	window              *mygo.Window
	tray                *mygo.Tray
	trayMenu            *mygo.Menu
	quitting, capturing bool
	startHidden         bool
	page                string
	settingsSection     int
	dark, frosted       bool
	glassTransparency   float64
	glassDensity        float64
	nativeTransparency  float64
	nativeGlassDensity  float64
	nativeFrosted       bool
	nativeDark          bool
	pins                map[*mygo.Window]*pinState
	autoPin             bool
	recordMic           bool
	recordSystem        bool
	actualPixels        bool
	scrollTask          *scrollTask
	recordTask          *recordingSession
	taskWindow          *mygo.Window
	closeTaskOutline    func()
	taskMessage         string
	lastVideo           string
	videoSaveDirectory  string
	latestVideoMenu     *mygo.MenuItem
	recordPreview       *recordPreview
	quitAfterPreview    bool
	pan                 ui.Point
	appearance          bool
	information         bool
	help                bool
	globalKey           string
	hotkeys             hotkeySettings
	hotkeyDraft         hotkeySettings
	hotkeyPath          string
	hotkeyMessage       string
	activeHotkeys       map[string]string
	settingsPath        string
	savedSettings       appearanceSettings
	zoom                float32
	bitmap              *ui.Bitmap
	data                []byte
	name, status        string
}

var icons = map[string]*ui.SVG{}

func init() {
	paths := map[string]string{
		"image":            `<rect x="3" y="3" width="18" height="18" rx="3"/><path d="m3 16 5-5 5 5 3-3 5 5"/><circle cx="16" cy="8" r="1.3"/>`,
		"pin":              `<path d="m9 3 12 12-4 1-3 5-3-3-5 3 3-5-3-3 5-3z"/>`,
		"tack":             `<path d="M9 3h6l-1 7 3 3v2H7v-2l3-3-1-7Z"/><path d="M12 15v6"/>`,
		"tack-filled":      `<path fill="currentColor" d="M9 3h6l-1 7 3 3v2H7v-2l3-3-1-7Z"/><path d="M12 15v6"/>`,
		"stop-recording":   `<rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" stroke="none"/>`,
		"cancel-recording": `<path d="m7 7 10 10M17 7 7 17"/>`,
		"settings":         `<path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="3" fill="currentColor"/><circle cx="15" cy="17" r="3" fill="currentColor"/>`,
		"long-capture":     `<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6m-6 4h6m-6 4h4"/>`,
		"video":            `<rect x="3" y="6" width="12" height="12" rx="2"/><path d="m15 10 6-3v10l-6-3"/>`,
		"folder":           `<path d="M3 7a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v9H3z"/>`,
		"copy":             `<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>`,
		"save":             `<path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5"/>`,
		"minus":            `<path d="M5 12h14"/>`,
		"plus":             `<path d="M5 12h14M12 5v14"/>`,
		"fit":              `<path d="M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5"/>`,
		"sun":              `<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M5 5l1.5 1.5m11 11L19 19M5 19l1.5-1.5m11-11L19 5"/>`,
	}
	for name, path := range paths {
		icons[name] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`))
	}
}

func newApp() *app {
	a := &app{page: "工作台", zoom: 1, autoPin: true, frosted: true, glassTransparency: defaultGlassTransparency, glassDensity: defaultGlassDensity, recordSystem: true, globalKey: "启动后注册", status: "就绪"}
	return a
}

func (a *app) theme() *ui.Theme {
	t := ui.LightTheme()
	if a.dark {
		t = ui.DarkTheme()
	}
	t.Accent = ui.Hex("#176680")
	t.AccentHover = ui.Hex("#115267")
	if a.dark {
		t.Accent = ui.Hex("#7CC8DA")
		t.AccentHover = ui.Hex("#9AD8E6")
		t.AccentText = ui.Hex("#14292E")
	}
	t.Radius = 5
	t.FontSize = 13
	switch runtime.GOOS {
	case "windows":
		t.Font = "Microsoft YaHei UI"
	case "darwin":
		t.Font = "PingFang SC"
	default:
		t.Font = "Noto Sans CJK SC"
	}
	if !a.dark {
		t.Background = ui.Hex("#F2F6F7")
		t.Text = ui.Hex("#25282D")
		t.TextMuted = ui.Hex("#525E65")
		t.Border = ui.Hex("#DEE1E5")
		t.Surface = ui.Hex("#FFFFFF")
		t.SurfaceHover = ui.Hex("#ECEEF1")
	}
	return t
}

func (a *app) button(c *ui.Context, key, icon, label string, primary bool) ui.Element {
	t := c.Theme()
	accessible := label
	if label == "" {
		accessible = map[string]string{"zoom-out": "缩小", "zoom-in": "放大"}[key]
	}
	e := ui.ButtonBase(c.Key(key)).Height(30).Shrink(0).Padding(5, 10).Radius(5).Label(accessible)
	if label == "" {
		e.Tooltip(accessible)
	}
	if primary {
		accent := t.Accent
		if a.hasGlassBackground(c) {
			accent = accent.Alpha(.86)
		}
		e.Background(accent).TextColor(t.AccentText)
	} else if e.Hovered() {
		e.Background(t.SurfaceHover)
	} else if a.hasGlassBackground(c) {
		e.Background(t.Surface).Border(1, t.Border)
	}
	e.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if icon != "" {
				ui.Icon(c, icons[icon]).Size(15, 15)
			}
			if label != "" {
				ui.Text(c, label).SingleLine()
			}
		})
	})
	return e
}

func (a *app) action(c *ui.Context, key, icon, label string, primary bool) bool {
	return a.button(c, key, icon, label, primary).Clicked()
}

func (a *app) material(e ui.Element, t *ui.Theme) {
	if a.frosted {
		e.Material(glass.Glass{Tint: t.Background.Alpha(.18)})
	} else {
		e.Background(t.Background).Border(1, t.Border)
	}
}

func (a *app) appearanceMenu(c *ui.Context, anchor ui.Element) {
	ui.PopoverBase(c, anchor, &a.appearance, func(panel ui.Element) {
		t := c.Theme()
		panel.Width(280).Padding(18).Radius(14).Margin(8, 0, 0)
		a.material(panel, t)
		panel.Children(func() {
			ui.Column(c).Gap(16).Children(func() {
				ui.Text(c, "外观").FontSize(16).Bold()
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "深色外观").Grow(1)
					ui.Switch(c.Key("appearance-dark"), &a.dark).Label("弹出面板深色外观")
				})
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "毛玻璃效果").Grow(1)
					ui.Switch(c.Key("appearance-glass"), &a.frosted).Label("弹出面板毛玻璃效果")
				})
				ui.Text(c, "系统支持时透出桌面；图片颜色不变。").FontSize(12).TextColor(t.TextMuted)
			})
		})
	})
}

func (a *app) view(c *ui.Context) {
	defer a.persistAppearance()
	a.setTheme(c)
	t := c.Theme()
	w, h := c.Size()
	windowBar := c.TitleBar()
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.open()
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		a.save()
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) {
		a.copy()
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) {
		a.pasteImage(false)
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) {
		a.pin()
	}
	if c.Shortcut(ui.Cmd, ui.KeyEqual) || c.Shortcut(ui.Cmd|ui.Shift, ui.KeyEqual) {
		a.zoom = min(3, a.zoom+.15)
	}
	if c.Shortcut(ui.Cmd, ui.KeyMinus) {
		a.zoom = max(.3, a.zoom-.15)
	}
	if c.Shortcut(ui.Cmd, ui.Key0) {
		a.zoom = 1
		a.actualPixels = false
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.page = "设置"
	}
	if c.Shortcut(0, ui.KeyF1) {
		a.help = !a.help
	}
	a.desktop(c, t, windowBar, h)
	a.shortcutHelp(c, w, h)
	a.syncAppearance()
}

func (a *app) shortcutHelp(c *ui.Context, width, height float32) {
	ui.DialogBase(c, &a.help, func(backdrop, panel ui.Element) {
		t := c.Theme()
		backdrop.Background(ui.Hex("#172A46").Alpha(.22))
		panel.Width(min(460, width-32)).Padding(20).Radius(16)
		a.material(panel, t)
		panel.Children(func() {
			ui.Column(c).Gap(14).Children(func() {
				ui.Text(c, "快捷键").FontSize(20).Bold()
				ui.Scroll(c).MaxHeight(max(180, height-180)).Children(func() {
					ui.Column(c).Gap(12).Children(func() {
						ui.Text(c, "全局快捷键").Bold()
						for _, action := range []struct{ id, label string }{{"capture", "区域截图"}, {"scroll", "滚动截图 / 完成"}, {"record", "录屏 / 停止"}, {"pin", "剪贴板贴图"}, {"show", "显示面板"}} {
							key := displayHotkey(a.activeHotkeys[action.id])
							if key == "" {
								key = "未启用"
							}
							ui.Row(c).Gap(16).Children(func() { ui.Text(c, action.label).Grow(1); ui.Text(c, key).TextColor(t.TextMuted) })
						}
						modifier := "Ctrl"
						if runtime.GOOS == "darwin" {
							modifier = "⌘"
						}
						for _, row := range [][2]string{{"O", "打开 PNG 图片"}, {"V", "粘贴图片并查看"}, {"C", "复制当前图片"}, {"S", "保存当前图片"}, {"P", "当前图片贴到桌面"}, {"+ / −", "放大 / 缩小"}, {"0", "适应画布"}, {",", "打开设置"}} {
							ui.Row(c).Gap(16).Children(func() { ui.Text(c, row[1]).Grow(1); ui.Text(c, modifier+" + "+row[0]).TextColor(t.TextMuted) })
						}
						ui.Text(c, "截图：松开鼠标自动复制，Esc / 右键取消。\n贴图：右键「保持显示」可固定，鼠标移到大图右上角也会显示图钉。拖动可移动，滚轮可缩放。\nCtrl/⌘+0 切换原始像素，Ctrl/⌘+C 复制，Esc 关闭。").FontSize(12).TextColor(t.TextMuted)
					})
				})
				if ui.Button(c, "知道了").Clicked() {
					a.help = false
				}
			})
		})
	})
}

func (a *app) pasteImage(pin bool) {
	data := mygo.Clipboard.ReadImage()
	if len(data) == 0 && pin && a.pinClipboardFile() {
		return
	}
	if len(data) == 0 {
		a.status = "请先复制图片或图片文件，再按贴图快捷键"
		if a.tray != nil {
			a.tray.SetToolTip("映刻 Screenpin · 剪贴板中没有图片")
		}
		if a.window != nil {
			a.page = "工作台"
			a.window.Show()
		}
		return
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		a.status = "无法读取剪贴板图片：" + err.Error()
		return
	}
	if pin {
		a.pinAsset(ui.NewBitmap(img).Pixelated(), data, "剪贴板图片.png", true)
		return
	}
	a.data = data
	a.bitmap = ui.NewBitmap(img).Pixelated()
	a.name = "剪贴板图片.png"
	a.zoom = 1
	a.actualPixels = false
	a.page = "图片"
	a.status = "已粘贴剪贴板图片"
}

func (a *app) registerShortcuts() {
	a.startHotkeys()
}

func (a *app) open() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.window, Title: "打开 PNG 图片", Filters: []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}}})
		if err != nil {
			a.window.Update(func() { a.status = "打开失败：" + err.Error() })
			return
		}
		if len(paths) == 0 {
			return
		}
		data, err := os.ReadFile(paths[0])
		if err != nil {
			a.window.Update(func() { a.status = "读取失败：" + err.Error() })
			return
		}
		img, err := png.Decode(bytes.NewReader(data))
		a.window.Update(func() {
			if err != nil {
				a.status = "图片格式无效：" + err.Error()
				return
			}
			a.data = data
			a.bitmap = ui.NewBitmap(img).Pixelated()
			a.name = filepath.Base(paths[0])
			a.zoom = 1
			a.actualPixels = false
			a.page = "图片"
			a.status = "已打开 " + a.name
		})
	}()
}

func (a *app) copy() {
	if !a.hasImage() {
		a.status = "请先打开或粘贴图片"
		return
	}
	if err := a.writeClipboardImage(a.data); err != nil {
		a.status = "复制失败：" + err.Error()
	} else {
		a.status = "已复制原尺寸图片"
	}
}

func (a *app) save() {
	if !a.hasImage() {
		a.status = "请先打开或粘贴图片"
		return
	}
	data := bytes.Clone(a.data)
	name := a.name
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.window, Title: "保存 PNG 图片", DefaultPath: name, Filters: []mygo.FileFilter{{Name: "PNG 图片", Extensions: []string{"png"}}}})
		if err == nil && path != "" {
			err = savePNG(path, data)
		}
		if path == "" && err == nil {
			return
		}
		a.window.Update(func() {
			if err != nil {
				a.status = "保存失败：" + err.Error()
			} else {
				a.status = "已保存到 " + path
			}
		})
	}()
}

func savePNG(path string, data []byte) error {
	return writeFileAtomic(path, data)
}

func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".screenpin-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (a *app) pin() {
	if !a.hasImage() {
		a.status = "请先打开或粘贴图片"
		return
	}
	a.pinAsset(a.bitmap, a.data, a.name, true)
}

func Run() {
	mygo.App.SetName(applicationName)
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	var running *app
	mygo.App.OnSecondInstance(func([]string, string) {
		if running != nil {
			running.showPanel()
		}
	})
	mygo.App.SetLanguages("zh-Hans")
	mygo.App.WhenReady(func() {
		a := newApp()
		running = a
		a.loadAppearance()
		a.lastVideo = latestRecording(a.recordingSaveDirectory())
		vibrancy := a.backdropMaterial()
		a.window = mygo.NewWindow(mygo.WindowOptions{
			Title: "映刻 Screenpin · 控制面板", Width: 820, Height: 620,
			MinWidth: 640, MinHeight: 540, TitleBarStyle: mygo.TitleBarHidden,
			TitleBarHeight: 38, Vibrancy: mygo.VibrancyAcrylic, Content: ui.View(a.view),
		})
		// Windows requires a material-capable composition window at creation,
		// even when the user initially chooses the opaque appearance.
		a.window.SetVibrancy(vibrancy)
		a.registerShortcuts()
		a.setupBackground()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
