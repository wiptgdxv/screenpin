//go:build !windows

package app

import (
	"fmt"
	"image"
)

func selectScreen(string) (screenSelection, error) {
	return screenSelection{}, fmt.Errorf("当前区域截图仅支持 Windows")
}
func grabScreenRegion(image.Rectangle) (*image.RGBA, error) {
	return nil, fmt.Errorf("当前区域截图仅支持 Windows")
}
func placeTaskWindow(uintptr, image.Rectangle, bool) bool { return false }
func recordingCue(bool)                                   {}
func cancelCapture()                                      {}
func showRegionOutline(image.Rectangle, bool) func()      { return func() {} }

func hideNativeWindowBorder(uintptr) {}
