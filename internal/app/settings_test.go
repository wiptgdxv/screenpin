package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestAppearanceConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Screenpin", "settings.json")
	got, err := readAppearance(path)
	if err != nil || got != (appearanceSettings{AutoPin: true, Frosted: true, RecordSystem: true, GlassTransparency: defaultGlassTransparency, GlassDensity: defaultGlassDensity}) {
		t.Fatalf("missing config: %+v, %v", got, err)
	}
	for _, want := range []appearanceSettings{{Dark: true}, {Frosted: true}, {StartHidden: true, Frosted: true, GlassTransparency: 73, GlassDensity: 67}} {
		if err := writeAppearance(path, want); err != nil {
			t.Fatal(err)
		}
		got, err = readAppearance(path)
		if err != nil || got != want {
			t.Fatalf("round trip: %+v, %v", got, err)
		}
	}
	if err := os.WriteFile(path, []byte(`{"dark":true,"glassEnabled":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = readAppearance(path)
	if err == nil || got != (appearanceSettings{AutoPin: true, Frosted: true, RecordSystem: true, GlassTransparency: defaultGlassTransparency, GlassDensity: defaultGlassDensity}) {
		t.Fatalf("invalid config partially applied: %+v, %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"dark":true,"frosted":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = readAppearance(path)
	if err != nil || !got.Dark || !got.Frosted {
		t.Fatalf("missing field lost default: %+v, %v", got, err)
	}
}

func TestAppearanceSwitchPersistsAndRestores(t *testing.T) {
	a := newTestApp()
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.savedSettings = appearanceSettings{Frosted: true}
	tt := ui.NewTester(a.view, 900, 600)
	tt.Frame()
	if err := tt.Click("外观"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("弹出面板深色外观"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("弹出面板毛玻璃效果"); err != nil {
		t.Fatal(err)
	}
	got, err := readAppearance(a.settingsPath)
	if err != nil || !got.Dark || got.Frosted {
		t.Fatalf("switches not saved: %+v, %v", got, err)
	}
}

func TestAppearanceSaveFailureKeepsActiveTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(path, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	a.settingsPath = filepath.Join(path, "settings.json")
	a.dark = true
	a.persistAppearance()
	if !a.dark || a.status == "" || a.savedSettings.Dark != true {
		t.Fatal("save failure lost active theme or feedback")
	}
}
