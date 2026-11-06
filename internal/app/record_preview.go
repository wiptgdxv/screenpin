package app

import (
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

//go:embed preview/record_preview.html
var recordPreviewHTML string

//go:embed preview/record_player.js
var recordPlayerJS string

type recordPreview struct {
	app           *app
	window        *mygo.Window
	path          string
	saved         string
	busy          bool
	confirming    bool
	presentSerial uint64
}

func (a *app) showRecordPreview(path string, audioDescription string) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	prefix := "/" + rand.Text() + "/"
	origin := "http://" + listener.Addr().String()
	address := origin + prefix
	class, material := "", mygo.VibrancyNone
	if a.frosted {
		class, material = "glass", mygo.VibrancyAcrylic
	}
	tint := fmt.Sprintf("--glass-tint:%.0f%%;--glass-stage-tint:%.2f;--glass-density:%.0f%%", 100*a.glassTint(.34), a.glassTint(.20), 100*a.glassDensityFraction())
	page := strings.Replace(recordPreviewHTML, `<html lang="zh-CN">`, `<html lang="zh-CN" class="`+class+`" data-audio="`+html.EscapeString(audioDescription)+`" style="`+tint+`">`, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case prefix:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, page)
		case prefix + "player.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, recordPlayerJS)
		case prefix + "video":
			w.Header().Set("Content-Type", "video/mp4")
			http.ServeFile(w, r, path)
		default:
			http.NotFound(w, r)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	p := &recordPreview{app: a, path: path}
	p.window = mygo.NewWindow(mygo.WindowOptions{
		Title: "录屏预览", URL: address, Width: 900, Height: 650, MinWidth: 550, MinHeight: 420,
		Frameless: true, HideWindowButtons: true, CornerRadius: 10, Vibrancy: material, BackgroundColor: "#e5edf0",
		// Only the local server hosting the built-in player may use title-bar dragging.
		Page: mygo.PageOptions{TrustedOrigins: []string{origin}},
	})
	hideNativeWindowBorder(p.window.NativeHandle())
	a.recordPreview = p
	p.window.Page().OnWillNavigate(func(event *mygo.NavigateEvent) {
		if event.URL == address {
			return
		}
		event.PreventDefault()
		target, _, _ := strings.Cut(event.URL, "?")
		switch target {
		case address + "save", address + "save-as":
			link, err := url.Parse(event.URL)
			if err != nil {
				return
			}
			edit, err := parseVideoEdit(link.Query())
			if err != nil {
				p.showStatus(err.Error())
				return
			}
			p.save(target == address+"save-as" || edit.format != "mp4", edit)
		case address + "minimize":
			p.window.Minimize()
		case address + "maximize":
			p.window.ToggleMaximize()
		case address + "close":
			p.window.Close()
		case address + "keep-preview":
			if p.confirming {
				p.confirming = false
				a.quitAfterPreview = false
			}
		case address + "discard-preview":
			if p.confirming && !p.busy && p.saved == "" {
				p.window.Destroy()
			}
		}
	})
	p.window.OnClose(func(event *mygo.CloseEvent) {
		if p.busy {
			event.PreventDefault()
			return
		}
		if p.saved != "" {
			return
		}
		event.PreventDefault()
		if p.confirming {
			return
		}
		p.confirming = true
		go func() {
			_, err := p.window.Page().Eval("window.screenpinConfirmClose()")
			if err != nil {
				mygo.RunOnMain(func() {
					if a.recordPreview == p {
						p.confirming = false
						a.quitAfterPreview = false
					}
				})
			}
		}()
	})
	p.window.OnClosed(func() {
		a.recordPreview = nil
		_ = server.Close()
		_ = os.Remove(path)
		if a.quitAfterPreview {
			a.quitAfterPreview = false
			mygo.App.Quit()
		}
	})
	p.present()
	return nil
}

func (p *recordPreview) present() {
	p.window.Show()
	// Finalization happens after the user action. Windows can refuse foreground
	// activation then; raise the preview without keeping it permanently on top.
	p.window.SetAlwaysOnTop(true)
	p.presentSerial++
	serial := p.presentSerial
	time.AfterFunc(3*time.Second, func() {
		mygo.RunOnMain(func() {
			if p.app.recordPreview == p && p.presentSerial == serial {
				p.window.SetAlwaysOnTop(false)
			}
		})
	})
}

func (p *recordPreview) showStatus(message string) {
	encoded, _ := json.Marshal(message)
	go func() {
		_, _ = p.window.Page().Eval("(() => { const line = document.getElementById('status'); line.title = line.textContent = " + string(encoded) + " })()")
	}()
}

func (p *recordPreview) save(saveAs bool, edit videoEdit) {
	if p.busy || p.confirming {
		return
	}
	p.busy = true
	p.showStatus("正在导出…")
	directory := p.app.recordingSaveDirectory()
	go func() {
		name := strings.TrimSuffix(filepath.Base(p.path), ".mp4")
		if edit.trimmed() {
			name += "_裁剪"
		}
		destination := filepath.Join(directory, name+"."+edit.format)
		var err error
		if !saveAs {
			if err = os.MkdirAll(directory, 0700); err == nil {
				destination, err = availableRecordingPath(destination)
			}
		}
		if saveAs {
			destination, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: p.window, Title: "导出 " + strings.ToUpper(edit.format), DefaultPath: destination, Filters: []mygo.FileFilter{{Name: strings.ToUpper(edit.format), Extensions: []string{edit.format}}}})
		}
		if err == nil && destination != "" {
			if !strings.EqualFold(filepath.Ext(destination), "."+edit.format) {
				destination += "." + edit.format
			}
			if edit.format == "mp4" && !edit.trimmed() {
				err = saveVideoCopy(p.path, destination)
			} else {
				err = exportRecording(p.path, destination, edit)
			}
		}
		mygo.RunOnMain(func() {
			p.busy = false
			if err != nil {
				p.showStatus("保存失败，请重试或另存。" + err.Error())
				return
			}
			if destination == "" {
				p.showStatus("已取消导出")
				return
			}
			p.saved = destination
			p.window.SetTitle("录屏预览 · 已保存")
			p.showStatus("已保存到 " + destination)
			a := p.app
			if edit.format != "mp4" {
				return
			}
			a.lastVideo = destination
			a.status = "录屏已保存：" + destination
			if a.latestVideoMenu != nil {
				a.latestVideoMenu.SetEnabled(true)
			}
			if a.tray != nil {
				a.tray.SetMenu(a.trayMenu)
				a.tray.SetToolTip("映刻 · 录屏已保存")
			}
			a.window.Invalidate()
		})
	}()
}

func availableRecordingPath(path string) (string, error) {
	candidate := path
	ext := filepath.Ext(path)
	for n := 2; ; n++ {
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(path, ext), n, ext)
	}
}

func saveVideoCopy(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".screenpin-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(output.Name(), destination)
}
