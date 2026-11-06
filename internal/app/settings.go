package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultGlassTransparency = 88.0
const maxGlassTransparency = 100.0
const defaultGlassDensity = 18.0

type appearanceSettings struct {
	VideoSaveDirectory string  `json:"videoSaveDirectory,omitempty"`
	GlassTransparency  float64 `json:"glassTransparency"`
	GlassDensity       float64 `json:"glassDensity"`
	AutoPin            bool    `json:"autoPin"`
	RecordMic          bool    `json:"recordMic,omitempty"`
	RecordSystem       bool    `json:"recordSystem"`
	StartHidden        bool    `json:"startHidden"`
	Dark               bool    `json:"dark"`
	Frosted            bool    `json:"glassEnabled"`
}

func readAppearance(path string) (appearanceSettings, error) {
	settings := appearanceSettings{AutoPin: true, Frosted: true, RecordSystem: true, GlassTransparency: defaultGlassTransparency, GlassDensity: defaultGlassDensity}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	// Decode into a copy so a malformed file cannot partially apply settings.
	decoded := settings
	if err := json.Unmarshal(data, &decoded); err != nil {
		return settings, fmt.Errorf("配置格式无效：%w", err)
	}
	decoded.GlassTransparency = clampGlassTransparency(decoded.GlassTransparency)
	decoded.GlassDensity = max(0, min(100, decoded.GlassDensity))
	return decoded, nil
}

func writeAppearance(path string, settings appearanceSettings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func (a *app) loadAppearance() {
	dir, err := os.UserConfigDir()
	if err != nil {
		a.status = "无法定位设置目录：" + err.Error()
		return
	}
	a.settingsPath = filepath.Join(dir, "Screenpin", "settings.json")
	settings, err := readAppearance(a.settingsPath)
	a.dark, a.frosted = settings.Dark, settings.Frosted
	a.glassTransparency, a.nativeTransparency = settings.GlassTransparency, settings.GlassTransparency
	a.glassDensity, a.nativeGlassDensity = settings.GlassDensity, settings.GlassDensity
	a.startHidden = settings.StartHidden
	a.autoPin = settings.AutoPin
	if filepath.IsAbs(settings.VideoSaveDirectory) {
		a.videoSaveDirectory = filepath.Clean(settings.VideoSaveDirectory)
	}
	a.recordMic = settings.RecordMic
	a.recordSystem = settings.RecordSystem
	a.nativeFrosted = a.frosted
	a.savedSettings = settings
	if err != nil {
		a.status = "读取设置失败，已使用默认外观：" + err.Error()
	}
}

func (a *app) persistAppearance() {
	settings := appearanceSettings{VideoSaveDirectory: a.videoSaveDirectory, GlassTransparency: a.glassTransparency, GlassDensity: a.glassDensity, Dark: a.dark, Frosted: a.frosted, StartHidden: a.startHidden, AutoPin: a.autoPin, RecordMic: a.recordMic, RecordSystem: a.recordSystem}
	if a.settingsPath == "" || settings == a.savedSettings {
		return
	}
	// Record the attempted state to avoid retrying disk writes every frame.
	// Toggling a setting again will retry and report any new failure.
	a.savedSettings = settings
	if err := writeAppearance(a.settingsPath, settings); err != nil {
		a.status = "当前设置未能保存：" + err.Error()
	}
}
