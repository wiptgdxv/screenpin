package app

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Keep one continuous native backdrop behind the entire window. Opaque
// child surfaces would hide it even if the system material is enabled.
func appearanceTheme(t *ui.Theme, translucent bool) *ui.Theme {
	if translucent {
		t.Surface = t.Surface.Alpha(.09)
		t.SurfaceHover = t.SurfaceHover.Alpha(.28)
		t.SurfacePressed = t.SurfacePressed.Alpha(.34)
		t.Border = t.Border.Alpha(.42)
	}
	return t
}

func (a *app) setTheme(c *ui.Context) {
	c.SetTheme(appearanceTheme(a.theme(), a.hasGlassBackground(c)))
}

const clearFadeStart = 30

func (a *app) clearBackground() bool {
	return usesClearBackdrop(a.frosted, a.glassTransparency)
}

func usesClearBackdrop(frosted bool, transparency float64) bool {
	return clearCompositionSupported() && (!frosted || transparency >= clearFadeStart)
}

func (a *app) hasGlassBackground(c *ui.Context) bool {
	return a.clearBackground() || (a.frosted && c.Vibrancy())
}

func (a *app) backdropMaterial() mygo.Vibrancy {
	if a.frosted && !a.clearBackground() {
		return mygo.VibrancyAcrylic
	}
	return mygo.VibrancyNone
}

func (a *app) clearTint() float32 {
	transparency := clampGlassTransparency(a.glassTransparency)
	if a.frosted {
		progress := float32((transparency - clearFadeStart) / (maxGlassTransparency - clearFadeStart))
		// Switch while the desktop contributes little color, then fade over the rest of the slider.
		startTint := .86 + .06*a.glassDensityFraction()
		return startTint + (.08-startTint)*progress
	}
	return 1 - .92*float32(transparency/maxGlassTransparency)
}

func clampGlassTransparency(value float64) float64 {
	return max(0, min(maxGlassTransparency, value))
}

// The system acrylic provides blur; the app controls its translucent tint and the matte overlay.
func (a *app) glassTint(minimum float64) float32 {
	visibility := clampGlassTransparency(a.glassTransparency) / maxGlassTransparency
	tint := 1 - (1-minimum)*visibility
	matte := .62 * math.Pow(float64(a.glassDensityFraction()), 1.4)
	return float32(tint + (1-tint)*matte)
}

func (a *app) glassDensityFraction() float32 {
	return float32(max(0, min(100, a.glassDensity)) / 100)
}

func (a *app) windowBackground(c *ui.Context) ui.Color {
	color := c.Theme().Background
	if a.clearBackground() {
		return color.Alpha(a.clearTint())
	}
	if a.frosted && c.Vibrancy() {
		if a.dark {
			return color.Mix(ui.Hex("#2C414B"), a.glassDensityFraction()).Alpha(a.glassTint(.10))
		}
		return color.Mix(ui.Hex("#D5E1E7"), a.glassDensityFraction()).Alpha(a.glassTint(.03))
	}
	return color
}

func (a *app) canvasBackground(c *ui.Context) ui.Color {
	color := ui.Hex("#E9EBEE")
	if a.dark {
		color = ui.Hex("#181B20")
	}
	if a.hasGlassBackground(c) {
		return color.Alpha(a.glassTint(.02) * .22)
	}
	return color
}

func (a *app) syncAppearance() {
	if a.window == nil || (a.nativeFrosted == a.frosted && a.nativeDark == a.dark && a.nativeTransparency == a.glassTransparency && a.nativeGlassDensity == a.glassDensity) {
		return
	}
	changedMaterial := a.nativeFrosted != a.frosted ||
		usesClearBackdrop(a.nativeFrosted, a.nativeTransparency) != a.clearBackground()
	a.nativeFrosted, a.nativeDark, a.nativeTransparency, a.nativeGlassDensity = a.frosted, a.dark, a.glassTransparency, a.glassDensity
	material := a.backdropMaterial()
	if changedMaterial {
		a.window.SetVibrancy(material)
	}
	a.window.Invalidate()
	for pinned := range a.pins {
		pinned.Invalidate()
	}
	if a.taskWindow != nil {
		if changedMaterial {
			a.taskWindow.SetVibrancy(material)
		}
		a.taskWindow.Invalidate()
	}
}
