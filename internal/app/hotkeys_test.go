package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeHotkeys(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{" ctrl + shift + a ", "CmdOrCtrl+Shift+A"}, {"Alt+Ctrl+2", "CmdOrCtrl+Alt+2"}, {"Ctrl+F12", "CmdOrCtrl+F12"}, {"F8", "F8"}, {"⌘+2", "CmdOrCtrl+2"}, {"", ""}} {
		got, err := normalizeHotkey(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q: %q %v", tc.in, got, err)
		}
	}
	for _, value := range []string{"A", "Ctrl+", "Ctrl+Ctrl+2", "Ctrl+F13", "Win+2", "Ctrl+Alt", "Ctrl+A+B"} {
		if _, err := normalizeHotkey(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if _, err := validateHotkeys(hotkeySettings{Pin: "Ctrl+2", Show: "CmdOrCtrl+2"}); err == nil {
		t.Fatal("duplicate action keys accepted")
	}
	if _, err := validateHotkeys(hotkeySettings{Capture: "Ctrl+1", Pin: "CmdOrCtrl+1"}); err == nil {
		t.Fatal("capture conflicts with pin")
	}
}

func TestOldHotkeyConfigurationKeepsCaptureDefault(t *testing.T) {
	settings := defaultHotkeys()
	if err := json.Unmarshal([]byte(`{"pin":"Ctrl+Alt+2","show":"F8"}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Capture != "CmdOrCtrl+1" {
		t.Fatal("new capture action lost its default during migration")
	}
	if err := json.Unmarshal([]byte(`{"capture":""}`), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Capture != "" {
		t.Fatal("explicitly disabled capture was restored")
	}
}

func TestHotkeyApplyTransaction(t *testing.T) {
	oldRegister, oldUnregister := registerHotkey, unregisterHotkey
	t.Cleanup(func() { registerHotkey = oldRegister; unregisterHotkey = oldUnregister })
	for _, scenario := range []string{"success", "occupied", "disk-failure", "disable"} {
		t.Run(scenario, func(t *testing.T) {
			registered := map[string]bool{"CmdOrCtrl+2": true}
			registerHotkey = func(key string, fn func()) error {
				if scenario == "occupied" && key == "CmdOrCtrl+Alt+0" {
					return fmt.Errorf("occupied")
				}
				registered[key] = true
				return nil
			}
			unregisterHotkey = func(key string) { delete(registered, key) }
			a := newApp()
			a.hotkeys = hotkeySettings{Pin: "CmdOrCtrl+2"}
			a.activeHotkeys = map[string]string{"pin": "CmdOrCtrl+2"}
			a.hotkeyPath = filepath.Join(t.TempDir(), "hotkeys.json")
			if scenario == "disk-failure" {
				if err := os.Mkdir(a.hotkeyPath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			a.hotkeyDraft = hotkeySettings{Pin: "Ctrl+Alt+9", Show: "Ctrl+Alt+0"}
			if scenario == "disable" {
				a.hotkeyDraft = hotkeySettings{}
			}
			a.applyHotkeys()
			if scenario == "occupied" || scenario == "disk-failure" {
				if len(registered) != 1 || !registered["CmdOrCtrl+2"] || a.hotkeys.Pin != "CmdOrCtrl+2" {
					t.Fatalf("failed change lost old bindings: %v %+v", registered, a.hotkeys)
				}
				return
			}
			if registered["CmdOrCtrl+2"] {
				t.Fatal("old binding leaked")
			}
			if scenario == "disable" && len(registered) != 0 {
				t.Fatal("disabled bindings remain")
			}
			data, err := os.ReadFile(a.hotkeyPath)
			if err != nil {
				t.Fatal(err)
			}
			var saved hotkeySettings
			if err = json.Unmarshal(data, &saved); err != nil || saved != a.hotkeys {
				t.Fatalf("saved config differs: %+v %v", saved, err)
			}
		})
	}
}
