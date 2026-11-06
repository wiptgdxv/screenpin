package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEmptyWorkspaceDoesNotActOnMissingImage(t *testing.T) {
	a := newApp()
	if a.hasImage() || !a.frosted {
		t.Fatal("startup should be an empty workspace with glass enabled")
	}
	tt := ui.NewTester(a.view, 640, 540)
	tt.Frame()
	if !tt.HasText("截图工作台") {
		t.Fatal("startup should show the launch panel")
	}
	a.page = "图片"
	tt.Frame()
	if !tt.HasText("打开图片，开始处理") {
		t.Fatal("empty workspace has no guidance")
	}
	for _, action := range []func(){a.copy, a.save, a.pin} {
		action()
		if a.status != "请先打开或粘贴图片" {
			t.Fatal("missing image action was not guarded")
		}
	}
}

func TestImagePreviewIsOpenedFromCurrentImage(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 640, 540)
	tt.Frame()
	for _, label := range []string{"图片", "查看图片"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("unexpected empty-workbench entry: %s", label)
		}
	}
	a.example()
	a.page = "工作台"
	tt.Frame()
	if err := tt.Click("查看图片"); err != nil {
		t.Fatal(err)
	}
	if a.page != "图片" || !tt.HasText(a.name) {
		t.Fatal("current image did not open in preview")
	}
	if err := tt.Click("工作台"); err != nil || a.page != "工作台" {
		t.Fatalf("could not return to workbench: %v", err)
	}
}

func TestLaunchPanelFitsMinimumWindow(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 640, 540)
	tt.Frame()
	for _, label := range []string{"开始截图", "贴图", "开始滚动", "开始录屏", "收起到托盘", "退出程序"} {
		r, ok := tt.Find(label)
		if !ok || r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > 641 || r.Y+r.H > 541 {
			t.Fatalf("launch action %s outside window: %+v", label, r)
		}
	}
}

func TestWorkspaceFitsDesktopSizes(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h float32
	}{{"minimum", 640, 540}, {"compact", 760, 650}, {"standard", 1080, 760}, {"wide", 1380, 850}} {
		t.Run(size.name, func(t *testing.T) {
			a := newTestApp()
			tt := ui.NewTester(a.view, int(size.w), int(size.h))
			tt.Frame()
			for _, label := range []string{"打开图片", "复制图片", "保存 PNG", "缩小", "放大", "适应", "贴到桌面"} {
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing action %s", label)
				}
				if r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > size.w+1 || r.Y+r.H > size.h+1 {
					t.Fatalf("action %s outside viewport: %+v", label, r)
				}
			}
			if err := tt.Click("放大"); err != nil {
				t.Fatal(err)
			}
			if a.zoom <= 1 {
				t.Fatal("zoom did not change")
			}
			if err := tt.Click("适应"); err != nil {
				t.Fatal(err)
			}
			if a.zoom != 1 {
				t.Fatal("fit did not reset zoom")
			}
		})
	}
}

func TestAppearancePopoverCanDisableGlass(t *testing.T) {
	for _, width := range []int{640, 1380} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("width-%d/dark-%v", width, dark), func(t *testing.T) {
				a := newTestApp()
				a.dark = dark
				tt := ui.NewTester(a.view, width, 650)
				tt.Frame()
				if err := tt.Click("外观"); err != nil {
					t.Fatal(err)
				}
				if !a.appearance {
					t.Fatal("appearance menu did not open")
				}
				label := "弹出面板毛玻璃效果"
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.H <= 0 || r.X < 0 || r.X+r.W > float32(width)+1 {
					t.Fatalf("popover switch outside window: %+v", r)
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if a.frosted {
					t.Fatal("glass did not turn off")
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if !a.frosted {
					t.Fatal("glass did not turn on")
				}
				tt.Key(0, ui.KeyEscape)
				if a.appearance {
					t.Fatal("Escape did not dismiss the appearance menu")
				}
			})
		}
	}
}

func TestSavePNGReplacesCompleteFile(t *testing.T) {
	a := newTestApp()
	path := filepath.Join(t.TempDir(), "图像.png")
	if err := os.WriteFile(path, []byte("previous content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := savePNG(path, a.data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, a.data) {
		t.Fatal("saved image differs from source")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files remain: %v", entries)
	}
}

func TestHeaderClearsSystemWindowButtons(t *testing.T) {
	for _, bar := range []ui.TitleBar{{Height: 48, Right: 138}, {Height: 48, Left: 84}, {Height: 48, Left: 40, Right: 80}} {
		for _, width := range []int{640, 1280} {
			a := newTestApp()
			tt := ui.NewTester(a.view, width, 650)
			tt.SetTitleBar(bar)
			tt.Frame()
			for _, label := range []string{"外观"} {
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.X < bar.Left || r.X+r.W > float32(width)-bar.Right+1 {
					t.Fatalf("%s overlaps window controls: width=%d bar=%+v rect=%+v", label, width, bar, r)
				}
			}
			if err := tt.Click("外观"); err != nil {
				t.Fatal(err)
			}
			if !a.appearance {
				t.Fatal("header button was treated as a drag region")
			}
		}
	}
}

func TestImageInformationIsAvailableOnDemand(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 900, 600)
	tt.Frame()
	if tt.HasText("800 × 500 像素") {
		t.Fatal("information panel should start closed")
	}
	if err := tt.Click("图像信息"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("800 × 500 像素") {
		t.Fatal("image information did not open")
	}
	tt.Key(0, ui.KeyEscape)
	if a.information {
		t.Fatal("information panel did not close")
	}
}

func TestWorkspaceKeyboardActions(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 900, 600)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEqual)
	if a.zoom <= 1 {
		t.Fatal("zoom-in shortcut did not work")
	}
	tt.Key(ui.Cmd, ui.Key0)
	if a.zoom != 1 {
		t.Fatal("fit shortcut did not reset zoom")
	}
	tt.Key(ui.Cmd, ui.KeyMinus)
	if a.zoom >= 1 {
		t.Fatal("zoom-out shortcut did not work")
	}
	tt.Key(ui.Cmd, ui.KeyComma)
	if a.page != "设置" {
		t.Fatal("settings shortcut did not work")
	}
	tt.Key(0, ui.KeyF1)
	if !a.help || !tt.HasText("快捷键") {
		t.Fatal("help shortcut did not work")
	}
	tt.Key(0, ui.KeyEscape)
	if a.help {
		t.Fatal("Escape did not dismiss shortcut help")
	}
}

func TestLatestRecordingSkipsIncompleteFiles(t *testing.T) {
	directory := t.TempDir()
	want := "录屏_20261105_130000.000_2.mp4"
	for _, name := range []string{"录屏_20261105_120000.000_1.mp4", want, "录屏_20261105_140000.000_3.partial.mp4", "screenpin-4.partial.mp4", "unrelated.mp4"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("video"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "录屏_20261105_150000.000_5.mp4"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := latestRecording(directory); got != filepath.Join(directory, want) {
		t.Fatalf("latestRecording = %q, want %q", got, want)
	}
}
