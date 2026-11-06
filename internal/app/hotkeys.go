package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var registerHotkey = mygo.GlobalShortcut.Register
var unregisterHotkey = mygo.GlobalShortcut.Unregister

type hotkeySettings struct {
	Capture string `json:"capture"`
	Scroll  string `json:"scroll"`
	Record  string `json:"record"`
	Pin     string `json:"pin"`
	Show    string `json:"show"`
}

func defaultHotkeys() hotkeySettings {
	return hotkeySettings{Capture: "CmdOrCtrl+1", Scroll: "CmdOrCtrl+3", Record: "CmdOrCtrl+4", Pin: "CmdOrCtrl+2", Show: "F8"}
}

func displayHotkey(key string) string {
	modifier := "Ctrl"
	if runtime.GOOS == "darwin" {
		modifier = "⌘"
	}
	return strings.ReplaceAll(key, "CmdOrCtrl", modifier)
}

func displayHotkeys(s hotkeySettings) hotkeySettings {
	return hotkeySettings{Scroll: displayHotkey(s.Scroll), Record: displayHotkey(s.Record), Capture: displayHotkey(s.Capture), Pin: displayHotkey(s.Pin), Show: displayHotkey(s.Show)}
}

// A deliberately small, portable accelerator vocabulary keeps validation and
// platform registration consistent. Empty strings explicitly disable an action.
func normalizeHotkey(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	parts := strings.Split(value, "+")
	mods := map[string]bool{}
	for _, part := range parts[:len(parts)-1] {
		mod := strings.ToLower(strings.TrimSpace(part))
		switch mod {
		case "ctrl", "control", "cmd", "command", "cmdorctrl", "⌘":
			mod = "CmdOrCtrl"
		case "alt":
			mod = "Alt"
		case "shift":
			mod = "Shift"
		default:
			return "", fmt.Errorf("不支持的修饰键：%s", part)
		}
		if mods[mod] {
			return "", fmt.Errorf("修饰键重复")
		}
		mods[mod] = true
	}
	key := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	valid := len(key) == 1 && ((key[0] >= 'A' && key[0] <= 'Z') || (key[0] >= '0' && key[0] <= '9'))
	functionKey := false
	for i := 1; i <= 12; i++ {
		if key == fmt.Sprintf("F%d", i) {
			valid = true
			functionKey = true
		}
	}
	if !valid || (len(mods) == 0 && !functionKey) {
		return "", fmt.Errorf("请输入 Ctrl+2 等组合键，或单独使用 F1–F12")
	}
	var result []string
	for _, mod := range []string{"CmdOrCtrl", "Alt", "Shift"} {
		if mods[mod] {
			result = append(result, mod)
		}
	}
	return strings.Join(append(result, key), "+"), nil
}

func hotkeyEntries(s hotkeySettings) []struct{ id, key string } {
	return []struct{ id, key string }{{"capture", s.Capture}, {"scroll", s.Scroll}, {"record", s.Record}, {"pin", s.Pin}, {"show", s.Show}}
}
func (s hotkeySettings) key(id string) string {
	for _, entry := range hotkeyEntries(s) {
		if entry.id == id {
			return entry.key
		}
	}
	return ""
}
func validateHotkeys(s hotkeySettings) (hotkeySettings, error) {
	capture, err := normalizeHotkey(s.Capture)
	if err != nil {
		return s, err
	}
	pin, err := normalizeHotkey(s.Pin)
	if err != nil {
		return s, err
	}
	show, err := normalizeHotkey(s.Show)
	if err != nil {
		return s, err
	}
	scroll, err := normalizeHotkey(s.Scroll)
	if err != nil {
		return s, err
	}
	record, err := normalizeHotkey(s.Record)
	if err != nil {
		return s, err
	}
	next := hotkeySettings{Capture: capture, Scroll: scroll, Record: record, Pin: pin, Show: show}
	seen := map[string]bool{}
	for _, entry := range hotkeyEntries(next) {
		if entry.key == "" {
			continue
		}
		if seen[entry.key] {
			return s, fmt.Errorf("两个动作不能使用相同快捷键")
		}
		seen[entry.key] = true
	}
	return next, nil
}

func (a *app) startHotkeys() {
	a.hotkeys = defaultHotkeys()
	if dir, err := os.UserConfigDir(); err == nil {
		a.hotkeyPath = filepath.Join(dir, "Screenpin", "hotkeys.json")
		if data, err := os.ReadFile(a.hotkeyPath); err == nil {
			candidate := a.hotkeys
			if err = json.Unmarshal(data, &candidate); err == nil {
				candidate, err = validateHotkeys(candidate)
			}
			if err == nil {
				a.hotkeys = candidate
			} else {
				a.hotkeyMessage = "快捷键配置无效，已使用默认值：" + err.Error()
			}
		} else if !os.IsNotExist(err) {
			a.hotkeyMessage = "读取快捷键失败：" + err.Error()
		}
	}
	a.hotkeyDraft = displayHotkeys(a.hotkeys)
	a.activeHotkeys = map[string]string{}
	for _, entry := range hotkeyEntries(a.hotkeys) {
		if entry.key == "" {
			continue
		}
		if err := registerHotkey(entry.key, a.hotkeyCallback(entry.id)); err != nil {
			log.Printf("register shortcut %s: %v", entry.id, err)
			if a.hotkeyMessage == "" {
				a.hotkeyMessage = "部分快捷键无法启用。请检查是否被其他程序占用，或更换组合键。"
			}
		} else {
			a.activeHotkeys[entry.id] = entry.key
		}
	}
	a.updateGlobalKey()
	mygo.App.OnQuit(func() {
		for _, key := range a.activeHotkeys {
			unregisterHotkey(key)
		}
	})
}

func (a *app) hotkeyCallback(id string) func() {
	return func() {
		mygo.RunOnMain(func() {
			if id == "capture" {
				a.capture()
			} else if id == "scroll" {
				a.scrollCapture()
			} else if id == "record" {
				a.recordVideo()
			} else if id == "pin" {
				a.pasteImage(true)
			} else {
				a.window.Show()
				a.page = "工作台"
			}
			a.window.Invalidate()
		})
	}
}

func (a *app) updateGlobalKey() {
	a.globalKey = a.activeHotkeys["pin"]
	if a.globalKey == "" {
		if a.hotkeys.Pin == "" {
			a.globalKey = "已停用"
		} else {
			a.globalKey = "未注册（可能被其他应用占用）"
		}
	}
}

func (a *app) applyHotkeys() {
	next, err := validateHotkeys(a.hotkeyDraft)
	if err != nil {
		a.hotkeyMessage = err.Error()
		return
	}
	if a.hotkeyPath == "" {
		a.hotkeyMessage = "无法定位快捷键配置目录"
		return
	}
	// Acquire new bindings before releasing old ones. Failed changes preserve
	// both the working registrations and the on-disk configuration.
	acquired := map[string]bool{}
	rollback := func() {
		for key := range acquired {
			unregisterHotkey(key)
		}
	}
	for _, entry := range hotkeyEntries(next) {
		if entry.key == "" {
			continue
		}
		for id, key := range a.activeHotkeys {
			if id != entry.id && key == entry.key {
				rollback()
				a.hotkeyMessage = "该组合键仍用于另一动作，请先清空并应用该动作"
				return
			}
		}
		if a.activeHotkeys[entry.id] == entry.key {
			continue
		}
		if err = registerHotkey(entry.key, a.hotkeyCallback(entry.id)); err != nil {
			rollback()
			log.Printf("apply shortcut %s: %v", entry.id, err)
			a.hotkeyMessage = displayHotkey(entry.key) + " 无法启用，请更换组合键。原快捷键仍可使用。"
			return
		}
		acquired[entry.key] = true
	}
	data, _ := json.MarshalIndent(next, "", "  ")
	if err = os.MkdirAll(filepath.Dir(a.hotkeyPath), 0700); err == nil {
		err = writeFileAtomic(a.hotkeyPath, append(data, '\n'))
	}
	if err != nil {
		rollback()
		a.hotkeyMessage = "保存失败，原设置已保留：" + err.Error()
		return
	}
	for id, key := range a.activeHotkeys {
		if key != next.key(id) {
			unregisterHotkey(key)
		}
	}
	a.activeHotkeys = map[string]string{}
	for _, entry := range hotkeyEntries(next) {
		if entry.key != "" {
			a.activeHotkeys[entry.id] = entry.key
		}
	}
	a.hotkeys = next
	a.hotkeyDraft = displayHotkeys(next)
	a.updateGlobalKey()
	a.hotkeyMessage = "快捷键已应用并保存"
}

func (a *app) hotkeySettingsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "全局快捷键").FontSize(14).Bold()
	ui.Text(c, "在其他应用中也能使用。留空可停用，修改后点击应用。").FontSize(12).TextColor(t.TextMuted)
	for _, row := range []struct {
		label string
		value *string
		id    string
	}{{"区域截图", &a.hotkeyDraft.Capture, "capture"}, {"滚动截图 / 完成", &a.hotkeyDraft.Scroll, "scroll"}, {"录屏 / 停止", &a.hotkeyDraft.Record, "record"}, {"剪贴板图片贴图", &a.hotkeyDraft.Pin, "pin"}, {"显示面板", &a.hotkeyDraft.Show, "show"}} {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
			state := "已停用"
			if key := a.activeHotkeys[row.id]; key != "" {
				state = "已启用 · " + displayHotkey(key)
			} else if a.hotkeys.key(row.id) != "" {
				state = "无法启用，请更换组合键"
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() { ui.Text(c, row.label); ui.Text(c, state).FontSize(11).TextColor(t.TextMuted) })
			ui.TextInput(c.Key("hotkey-"+row.id), row.value).Label(row.label + "快捷键").Width(180).Height(32)
		})
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if a.action(c, "hotkeys-default", "", "恢复默认快捷键", false) {
			a.hotkeyDraft = displayHotkeys(defaultHotkeys())
			a.hotkeyMessage = "默认快捷键已填入，点击应用生效"
		}
		ui.Box(c).Grow(1)
		if a.action(c, "hotkeys-apply", "", "应用快捷键", true) {
			a.applyHotkeys()
		}
	})
	if a.hotkeyMessage != "" {
		ui.Text(c, a.hotkeyMessage).FontSize(12).TextColor(t.TextMuted)
	}
}
