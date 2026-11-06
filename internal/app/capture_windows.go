//go:build windows

package app

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var capUser = windows.NewLazySystemDLL("user32.dll")
var capGDI = windows.NewLazySystemDLL("gdi32.dll")
var capKernel = windows.NewLazySystemDLL("kernel32.dll")
var capClassOnce sync.Once
var capClassErr error
var capSession *regionOverlay // accessed only by the locked capture thread
var capWindow atomic.Uintptr
var capLock sync.Mutex

type capRect struct{ Left, Top, Right, Bottom int32 }
type capPoint struct{ X, Y int32 }
type capMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          capPoint
	Private        uint32
}
type capClass struct {
	Style                              uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
}
type capPaint struct {
	DC                 uintptr
	Erase              int32
	Rect               capRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type capBitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	Used, Important        uint32
	Color                  uint32
}
type capFrame struct {
	dc, bitmap, old uintptr
	pixels          []byte
	width, height   int
}
type regionOverlay struct {
	source, dim *capFrame
	banner      *capFrame
	start, end  image.Point
	dragging    bool
	selected    image.Rectangle
	bounds      image.Rectangle
	hint        string
	releaseHint string
}

//go:uintptrescapes
func capCall(name string, args ...uintptr) uintptr {
	v, _, _ := capUser.NewProc(name).Call(args...)
	return v
}

//go:uintptrescapes
func gdiCall(name string, args ...uintptr) uintptr {
	v, _, _ := capGDI.NewProc(name).Call(args...)
	return v
}
func capWide(s string) *uint16 { return windows.StringToUTF16Ptr(s) }

func newCapFrame(screen uintptr, width, height int) (*capFrame, error) {
	f := &capFrame{width: width, height: height}
	f.dc = gdiCall("CreateCompatibleDC", screen)
	if f.dc == 0 {
		return nil, fmt.Errorf("无法创建截图画布")
	}
	info := capBitmapInfo{Size: 40, Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	f.bitmap = gdiCall("CreateDIBSection", screen, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if f.bitmap == 0 || bits == nil {
		f.close()
		return nil, fmt.Errorf("无法分配截图内存")
	}
	f.old = gdiCall("SelectObject", f.dc, f.bitmap)
	f.pixels = unsafe.Slice((*byte)(bits), width*height*4)
	return f, nil
}

func (f *capFrame) close() {
	if f.old != 0 {
		gdiCall("SelectObject", f.dc, f.old)
	}
	if f.bitmap != 0 {
		gdiCall("DeleteObject", f.bitmap)
	}
	if f.dc != 0 {
		gdiCall("DeleteDC", f.dc)
	}
}

func capWndProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	s := capSession
	if s == nil {
		return capCall("DefWindowProcW", hwnd, uintptr(message), wparam, lparam)
	}
	point := func() image.Point { return image.Pt(int(int16(lparam&0xffff)), int(int16((lparam>>16)&0xffff))) }
	switch message {
	case 0x0014:
		return 1 // WM_ERASEBKGND: snapshot paints the complete window
	case 0x000f: // WM_PAINT
		var paint capPaint
		dc := capCall("BeginPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		bannerX := (s.bounds.Dx() - s.banner.width) / 2
		const bannerY = 24
		saved := gdiCall("SaveDC", dc)
		if saved != 0 {
			gdiCall("ExcludeClipRect", dc, uintptr(bannerX), bannerY, uintptr(bannerX+s.banner.width), uintptr(bannerY+s.banner.height))
		}
		gdiCall("BitBlt", dc, 0, 0, uintptr(s.bounds.Dx()), uintptr(s.bounds.Dy()), s.dim.dc, 0, 0, 0x00cc0020)
		rect := selectionRect(s.start, s.end, s.bounds)
		if s.dragging && !rect.Empty() {
			gdiCall("BitBlt", dc, uintptr(rect.Min.X), uintptr(rect.Min.Y), uintptr(rect.Dx()), uintptr(rect.Dy()), s.source.dc, uintptr(rect.Min.X), uintptr(rect.Min.Y), 0x00cc0020)
			pen := gdiCall("CreatePen", 0, 2, 0x00eb6325)
			oldPen := gdiCall("SelectObject", dc, pen)
			oldBrush := gdiCall("SelectObject", dc, gdiCall("GetStockObject", 5))
			gdiCall("Rectangle", dc, uintptr(rect.Min.X), uintptr(rect.Min.Y), uintptr(rect.Max.X), uintptr(rect.Max.Y))
			gdiCall("SelectObject", dc, oldBrush)
			gdiCall("SelectObject", dc, oldPen)
			gdiCall("DeleteObject", pen)
		}
		if saved != 0 {
			gdiCall("RestoreDC", dc, saved)
		}
		bannerDC := s.banner.dc
		hint := capRect{Right: int32(s.banner.width), Bottom: int32(s.banner.height)}
		brush := gdiCall("CreateSolidBrush", 0x0025201c)
		capCall("FillRect", bannerDC, uintptr(unsafe.Pointer(&hint)), brush)
		gdiCall("DeleteObject", brush)
		oldFont := gdiCall("SelectObject", bannerDC, gdiCall("GetStockObject", 17))
		gdiCall("SetTextColor", bannerDC, 0x00ffffff)
		gdiCall("SetBkMode", bannerDC, 1)
		label := s.hint
		if s.dragging {
			widthLabel := capRect{Left: 12, Right: 54, Bottom: hint.Bottom}
			dimensionSeparator := capRect{Left: 54, Right: 72, Bottom: hint.Bottom}
			heightLabel := capRect{Left: 72, Right: 160, Bottom: hint.Bottom}
			capCall("DrawTextW", bannerDC, uintptr(unsafe.Pointer(capWide(fmt.Sprint(rect.Dx())))), ^uintptr(0), uintptr(unsafe.Pointer(&widthLabel)), 0x26)
			capCall("DrawTextW", bannerDC, uintptr(unsafe.Pointer(capWide("×"))), ^uintptr(0), uintptr(unsafe.Pointer(&dimensionSeparator)), 0x25)
			capCall("DrawTextW", bannerDC, uintptr(unsafe.Pointer(capWide(fmt.Sprintf("%d 像素", rect.Dy())))), ^uintptr(0), uintptr(unsafe.Pointer(&heightLabel)), 0x24)
			hint.Left = 164
			label = s.releaseHint + " · Esc 取消"
		}
		capCall("DrawTextW", bannerDC, uintptr(unsafe.Pointer(capWide(label))), ^uintptr(0), uintptr(unsafe.Pointer(&hint)), 0x25)
		gdiCall("SelectObject", bannerDC, oldFont)
		gdiCall("BitBlt", dc, uintptr(bannerX), bannerY, uintptr(s.banner.width), uintptr(s.banner.height), bannerDC, 0, 0, 0x00cc0020)
		capCall("EndPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		return 0
	case 0x0201: // WM_LBUTTONDOWN
		s.start = point()
		s.end = s.start
		s.dragging = true
		capCall("SetCapture", hwnd)
		return 0
	case 0x0200: // WM_MOUSEMOVE
		if s.dragging {
			s.end = point()
			capCall("InvalidateRect", hwnd, 0, 0)
		}
		return 0
	case 0x0202: // WM_LBUTTONUP
		if s.dragging {
			s.end = point()
			s.selected = selectionRect(s.start, s.end, s.bounds)
			capCall("ReleaseCapture")
			capCall("DestroyWindow", hwnd)
		}
		return 0
	case 0x0204, 0x0010, 0x007e: // right click, close, display configuration change
		s.selected = image.Rectangle{}
		capCall("DestroyWindow", hwnd)
		return 0
	case 0x0100:
		if wparam == 27 {
			s.selected = image.Rectangle{}
			capCall("DestroyWindow", hwnd)
			return 0
		}
	case 0x0002: // WM_DESTROY
		capWindow.Store(0)
		capCall("PostQuitMessage", 0)
		return 0
	}
	return capCall("DefWindowProcW", hwnd, uintptr(message), wparam, lparam)
}

func selectScreen(hint string) (screenSelection, error) {
	if !capLock.TryLock() {
		return screenSelection{}, fmt.Errorf("正在截图")
	}
	defer capLock.Unlock()
	previousWindow := capCall("GetForegroundWindow")
	defer func() {
		if previousWindow != 0 {
			capCall("SetForegroundWindow", previousWindow)
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Physical pixels across monitors, including monitors left/above the primary.
	if proc := capUser.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3))
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	x := int(int32(capCall("GetSystemMetrics", 76)))
	y := int(int32(capCall("GetSystemMetrics", 77)))
	width := int(capCall("GetSystemMetrics", 78))
	height := int(capCall("GetSystemMetrics", 79))
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 64*1024*1024 {
		return screenSelection{}, fmt.Errorf("桌面尺寸无效或超过截图内存限制")
	}
	screen := capCall("GetDC", 0)
	if screen == 0 {
		return screenSelection{}, fmt.Errorf("无法读取桌面")
	}
	defer capCall("ReleaseDC", 0, screen)
	source, err := newCapFrame(screen, width, height)
	if err != nil {
		return screenSelection{}, err
	}
	defer source.close()
	if gdiCall("BitBlt", source.dc, 0, 0, uintptr(width), uintptr(height), screen, uintptr(x), uintptr(y), 0x40cc0020) == 0 {
		return screenSelection{}, fmt.Errorf("桌面捕获失败")
	}
	gdiCall("GdiFlush")
	dim, err := newCapFrame(screen, width, height)
	if err != nil {
		return screenSelection{}, err
	}
	defer dim.close()
	banner, err := newCapFrame(screen, min(width, 440), 38)
	if err != nil {
		return screenSelection{}, err
	}
	defer banner.close()
	for i := 0; i < len(source.pixels); i += 4 {
		for channel := 0; channel < 3; channel++ {
			dim.pixels[i+channel] = source.pixels[i+channel] / 2
		}
		dim.pixels[i+3] = 255
	}
	instance, _, _ := capKernel.NewProc("GetModuleHandleW").Call(0)
	capClassOnce.Do(func() {
		class := capClass{Proc: syscall.NewCallback(capWndProc), Instance: instance, Cursor: capCall("LoadCursorW", 0, 32515), ClassName: capWide("ScreenpinCapture")}
		if capCall("RegisterClassW", uintptr(unsafe.Pointer(&class))) == 0 {
			capClassErr = fmt.Errorf("无法创建截图覆盖层")
		}
	})
	if capClassErr != nil {
		return screenSelection{}, capClassErr
	}
	session := &regionOverlay{source: source, dim: dim, banner: banner, bounds: image.Rect(0, 0, width, height), hint: hint}
	session.releaseHint = "松开复制"
	if strings.Contains(hint, "滚动") {
		session.releaseHint = "松开开始滚动截图"
	}
	if strings.Contains(hint, "录屏") {
		session.releaseHint = "松开开始录屏"
	}
	capSession = session
	defer func() { capSession = nil }()
	hwnd := capCall("CreateWindowExW", 0x88, uintptr(unsafe.Pointer(capWide("ScreenpinCapture"))), uintptr(unsafe.Pointer(capWide("Screenpin · 区域截图"))), 0x80000000, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, instance, 0)
	if hwnd == 0 {
		return screenSelection{}, fmt.Errorf("无法显示截图覆盖层")
	}
	capWindow.Store(hwnd)
	defer func() {
		if capWindow.Swap(0) != 0 {
			capCall("DestroyWindow", hwnd)
		}
	}()
	capCall("ShowWindow", hwnd, 5)
	capCall("SetForegroundWindow", hwnd)
	capCall("SetFocus", hwnd)
	capCall("UpdateWindow", hwnd)
	var message capMessage
	for {
		result := int32(capCall("GetMessageW", uintptr(unsafe.Pointer(&message)), 0, 0, 0))
		if result == 0 {
			break
		}
		if result == -1 {
			return screenSelection{}, fmt.Errorf("截图窗口消息处理失败")
		}
		capCall("TranslateMessage", uintptr(unsafe.Pointer(&message)))
		capCall("DispatchMessageW", uintptr(unsafe.Pointer(&message)))
	}
	rect := session.selected
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return screenSelection{}, errCaptureCancelled
	}
	img := rgbaFromBGRA(source.pixels, width*4, rect)
	return screenSelection{Image: img, Bounds: rect.Add(image.Pt(x, y))}, nil
}

func cancelCapture() {
	if hwnd := capWindow.Load(); hwnd != 0 {
		capCall("PostMessageW", hwnd, 0x0010, 0, 0)
	}
}

func grabScreenRegion(rect image.Rectangle) (*image.RGBA, error) {
	if rect.Empty() || int64(rect.Dx())*int64(rect.Dy()) > 64*1024*1024 {
		return nil, fmt.Errorf("选区尺寸无效")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if proc := capUser.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3))
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	screen := capCall("GetDC", 0)
	if screen == 0 {
		return nil, fmt.Errorf("无法读取桌面")
	}
	defer capCall("ReleaseDC", 0, screen)
	frame, err := newCapFrame(screen, rect.Dx(), rect.Dy())
	if err != nil {
		return nil, err
	}
	defer frame.close()
	if gdiCall("BitBlt", frame.dc, 0, 0, uintptr(rect.Dx()), uintptr(rect.Dy()), screen, uintptr(rect.Min.X), uintptr(rect.Min.Y), 0x40cc0020) == 0 {
		return nil, fmt.Errorf("选区抓取失败")
	}
	gdiCall("GdiFlush")
	return rgbaFromBGRA(frame.pixels, frame.width*4, image.Rect(0, 0, frame.width, frame.height)), nil
}

func virtualScreenBounds() image.Rectangle {
	left, top := int(int32(capCall("GetSystemMetrics", 76))), int(int32(capCall("GetSystemMetrics", 77)))
	return image.Rect(left, top, left+int(capCall("GetSystemMetrics", 78)), top+int(capCall("GetSystemMetrics", 79)))
}

func excludeFromCapture(window uintptr) bool {
	return windows.RtlGetVersion().BuildNumber >= 19041 && capCall("SetWindowDisplayAffinity", window, 0x11) != 0
}

func recordingCue(start bool) {
	sound := uintptr(0x40) // MB_ICONASTERISK
	if !start {
		sound = 0x20 // MB_ICONQUESTION
	}
	capCall("MessageBeep", sound)
}

func placeTaskWindow(window uintptr, rect image.Rectangle, recording bool) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if proc := capUser.NewProc("SetThreadDpiAwarenessContext"); proc.Find() == nil {
		previous, _, _ := proc.Call(^uintptr(3))
		if previous != 0 {
			defer proc.Call(previous)
		}
	}
	var bounds capRect
	if capCall("GetWindowRect", window, uintptr(unsafe.Pointer(&bounds))) == 0 {
		return false
	}
	width, height := int(bounds.Right-bounds.Left), int(bounds.Bottom-bounds.Top)
	screen := virtualScreenBounds()
	excluded := excludeFromCapture(window)
	const outlineGap = 6
	gap := outlineGap
	if !recording {
		policy := uint32(1) // DWMNCRP_DISABLED
		windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(window, 2, uintptr(unsafe.Pointer(&policy)), unsafe.Sizeof(policy))
		if !excluded {
			gap = 80 // Keep the compositor's shadow clear when capture exclusion is unavailable.
		}
	}
	x := max(screen.Min.X, min(rect.Min.X, screen.Max.X-width))
	y := max(screen.Min.Y, min(rect.Min.Y, screen.Max.Y-height))
	positions := []image.Point{
		image.Pt(x, rect.Max.Y+gap), image.Pt(x, rect.Min.Y-height-gap),
		image.Pt(rect.Max.X+gap, y), image.Pt(rect.Min.X-width-gap, y),
	}
	// The control is immovable by the user. Skip WM_WINDOWPOSCHANGING
	// so MyGo does not suppress this programmatic move.
	const moveFlags = 0x415 // NOSENDCHANGING | NOACTIVATE | NOZORDER | NOSIZE
	for _, pos := range positions {
		if (image.Rectangle{Min: pos, Max: pos.Add(image.Pt(width, height))}).In(screen) {
			return capCall("SetWindowPos", window, 0, uintptr(pos.X), uintptr(pos.Y), 0, 0, moveFlags) != 0
		}
	}
	if excluded {
		x = max(screen.Min.X, min(rect.Max.X-width-outlineGap, screen.Max.X-width))
		y = max(screen.Min.Y, min(rect.Min.Y+outlineGap, screen.Max.Y-height))
		return capCall("SetWindowPos", window, 0, uintptr(x), uintptr(y), 0, 0, moveFlags) != 0
	}
	return false
}

func hideNativeWindowBorder(window uintptr) {
	if windows.RtlGetVersion().BuildNumber < 22000 {
		return
	}
	const borderColorAttribute = 34 // DWMWA_BORDER_COLOR
	color := uint32(0xfffffffe)     // DWMWA_COLOR_NONE
	dwm := windows.NewLazySystemDLL("dwmapi.dll")
	dwm.NewProc("DwmSetWindowAttribute").Call(window, borderColorAttribute, uintptr(unsafe.Pointer(&color)), unsafe.Sizeof(color))
}
