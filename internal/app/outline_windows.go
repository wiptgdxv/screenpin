//go:build windows

package app

import (
	"image"
	"sync"
	"syscall"
	"unsafe"
)

var outlineClassOnce sync.Once
var outlineClassReady bool

func outlineProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case 0x0084: // HTTRANSPARENT
		return ^uintptr(0)
	case 0x000f: // WM_PAINT
		var paint capPaint
		dc := capCall("BeginPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		brush := gdiCall("GetStockObject", 18) // DC_BRUSH
		gdiCall("SetDCBrushColor", dc, capCall("GetWindowLongPtrW", hwnd, ^uintptr(20)))
		capCall("FillRect", dc, uintptr(unsafe.Pointer(&paint.Rect)), brush)
		capCall("EndPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		return 0
	}
	return capCall("DefWindowProcW", hwnd, uintptr(message), wparam, lparam)
}

// Strips sit outside the selected pixels when there is room on screen.
func showRegionOutline(bounds image.Rectangle, recording bool) func() {
	if proc := capUser.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3))
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	instance := kernelCall("GetModuleHandleW", 0)
	outlineClassOnce.Do(func() {
		class := capClass{Proc: syscall.NewCallback(outlineProc), Instance: instance, ClassName: capWide("ScreenpinRegionOutline")}
		outlineClassReady = capCall("RegisterClassW", uintptr(unsafe.Pointer(&class))) != 0
	})
	var windows []uintptr
	if outlineClassReady {
		color := uintptr(0x00eb6325)
		if recording {
			color = 0x003b3bef
		}
		thickness := 4
		if recording {
			thickness = 6
		}
		x, y := bounds.Min.X, bounds.Min.Y
		right, bottom := bounds.Max.X, bounds.Max.Y
		screen := virtualScreenBounds()
		for _, strip := range []image.Rectangle{
			image.Rect(x-thickness, y-thickness, right+thickness, y),
			image.Rect(x-thickness, bottom, right+thickness, bottom+thickness),
			image.Rect(x-thickness, y, x, bottom),
			image.Rect(right, y, right+thickness, bottom),
		} {
			needsExclusion := !strip.Overlaps(screen)
			if needsExclusion {
				if !recording {
					continue
				}
				shift := image.Pt(0, 0)
				switch {
				case strip.Max.Y <= screen.Min.Y:
					shift.Y = thickness
				case strip.Min.Y >= screen.Max.Y:
					shift.Y = -thickness
				case strip.Max.X <= screen.Min.X:
					shift.X = thickness
				case strip.Min.X >= screen.Max.X:
					shift.X = -thickness
				}
				strip = strip.Add(shift).Intersect(bounds)
			}
			const styles = 0x080800a8 // NOACTIVATE | LAYERED | TOOLWINDOW | TRANSPARENT | TOPMOST
			hwnd := capCall("CreateWindowExW", styles, uintptr(unsafe.Pointer(capWide("ScreenpinRegionOutline"))), uintptr(unsafe.Pointer(capWide("Screenpin 选区边框"))), 0x80000000, uintptr(strip.Min.X), uintptr(strip.Min.Y), uintptr(strip.Dx()), uintptr(strip.Dy()), 0, 0, instance, 0)
			if hwnd != 0 {
				if needsExclusion && !excludeFromCapture(hwnd) {
					capCall("DestroyWindow", hwnd)
					continue
				}
				windows = append(windows, hwnd)
				capCall("SetWindowLongPtrW", hwnd, ^uintptr(20), color)
				capCall("SetLayeredWindowAttributes", hwnd, 0, 255, 2)
				capCall("ShowWindow", hwnd, 4) // SW_SHOWNOACTIVATE
				capCall("UpdateWindow", hwnd)
			}
		}
	}
	return func() {
		for _, hwnd := range windows {
			capCall("DestroyWindow", hwnd)
		}
	}
}
