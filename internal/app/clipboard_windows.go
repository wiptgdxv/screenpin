//go:build windows

package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"
)

const clipboardPNGPrefix = "截图_"

func clipboardFileList(path string) []byte {
	const headerSize = 20 // DROPFILES
	name := utf16.Encode([]rune(path))
	data := make([]byte, headerSize+2*(len(name)+2))
	binary.LittleEndian.PutUint32(data, headerSize)
	binary.LittleEndian.PutUint32(data[16:], 1) // fWide
	for i, value := range name {
		binary.LittleEndian.PutUint16(data[headerSize+i*2:], value)
	}
	return data
}

func stageClipboardPNG(data []byte) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(cache, "Screenpin", "Clipboard")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, clipboardPNGPrefix+time.Now().Format("20060102_150405")+"_*.png")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(file.Name())
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return file.Name(), nil
}

// Expire only old snapshots after publishing their replacement. Keep recent
// ones available to Explorer if a paste was queued before the clipboard changed.
func pruneClipboardPNGs(directory string) {
	entries, _ := os.ReadDir(directory)
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, clipboardPNGPrefix) || !strings.HasSuffix(name, ".png") {
			continue
		}
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(directory, name))
		}
	}
}

func platformWriteClipboardImage(owner uintptr, data []byte) error {
	// Some Windows clients request CF_BITMAP directly from OLE instead of
	// accepting DIB/PNG or asking Win32 to synthesize a bitmap. Keep all three.
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	bounds := img.Bounds()
	if int64(bounds.Dx())*int64(bounds.Dy()) > 64*1024*1024 {
		return fmt.Errorf("图片超过剪贴板位图大小限制")
	}
	screen := capCall("GetDC", 0)
	if screen == 0 {
		return fmt.Errorf("无法创建剪贴板位图")
	}
	defer capCall("ReleaseDC", 0, screen)
	frame, err := newCapFrame(screen, bounds.Dx(), bounds.Dy())
	if err != nil {
		return err
	}
	defer frame.close()
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			offset := (y*bounds.Dx() + x) * 4
			// CF_BITMAP has no portable alpha. PNG retains the original alpha, while
			// this compatibility representation composites transparent pixels on white.
			frame.pixels[offset], frame.pixels[offset+1], frame.pixels[offset+2], frame.pixels[offset+3] = byte((b+0xffff-a)>>8), byte((g+0xffff-a)>>8), byte((r+0xffff-a)>>8), 255
		}
	}
	gdiCall("SelectObject", frame.dc, frame.old)
	frame.old = 0

	dib := make([]byte, 40+len(frame.pixels))
	binary.LittleEndian.PutUint32(dib, 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(bounds.Dx()))
	binary.LittleEndian.PutUint32(dib[8:], uint32(-int32(bounds.Dy())))
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	copy(dib[40:], frame.pixels)
	pngFormat := capCall("RegisterClipboardFormatW", uintptr(unsafe.Pointer(capWide("PNG"))))
	mimeFormat := capCall("RegisterClipboardFormatW", uintptr(unsafe.Pointer(capWide("image/png"))))
	if pngFormat == 0 || mimeFormat == 0 {
		return fmt.Errorf("无法注册图片剪贴板格式")
	}
	type clipboardItem struct {
		format uintptr
		data   []byte
		handle uintptr
	}
	items := []clipboardItem{{format: pngFormat, data: data}, {format: mimeFormat, data: data}, {format: 8, data: dib}}
	var filePublished bool
	if path, err := stageClipboardPNG(data); err == nil {
		defer func() {
			if filePublished {
				go pruneClipboardPNGs(filepath.Dir(path))
			} else {
				_ = os.Remove(path)
			}
		}()
		if dropEffect := capCall("RegisterClipboardFormatW", uintptr(unsafe.Pointer(capWide("Preferred DropEffect")))); dropEffect != 0 {
			items = append(items, clipboardItem{format: dropEffect, data: []byte{1, 0, 0, 0}}) // DROPEFFECT_COPY
		}
		items = append(items, clipboardItem{format: 15, data: clipboardFileList(path)}) // CF_HDROP
	}
	defer func() {
		for _, item := range items {
			if item.handle != 0 {
				kernelCall("GlobalFree", item.handle)
			}
		}
	}()
	for i := range items {
		item := &items[i]
		item.handle = kernelCall("GlobalAlloc", 2, uintptr(len(item.data)))
		if item.handle == 0 {
			return fmt.Errorf("分配剪贴板内存失败")
		}
		memory := kernelCall("GlobalLock", item.handle)
		if memory == 0 {
			return fmt.Errorf("锁定剪贴板内存失败")
		}
		kernelCall("RtlMoveMemory", memory, uintptr(unsafe.Pointer(&item.data[0])), uintptr(len(item.data)))
		kernelCall("GlobalUnlock", item.handle)
	}
	opened := false
	for attempt := 0; attempt < 6; attempt++ {
		if capCall("OpenClipboard", owner) != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		return fmt.Errorf("剪贴板暂时被其他程序占用")
	}
	defer capCall("CloseClipboard")
	if capCall("EmptyClipboard") == 0 {
		return fmt.Errorf("无法更新剪贴板")
	}
	for i := range items {
		item := &items[i]
		if item.format == 15 {
			// Publish the file after the bitmap formats so image targets get preference.
			continue
		}
		if capCall("SetClipboardData", item.format, item.handle) == 0 {
			return fmt.Errorf("写入图片剪贴板格式失败")
		}
		item.handle = 0
	}
	if capCall("SetClipboardData", 2, frame.bitmap) == 0 {
		return fmt.Errorf("写入兼容位图格式失败")
	}
	frame.bitmap = 0 // ownership transferred to the system
	last := &items[len(items)-1]
	if last.format == 15 && capCall("SetClipboardData", last.format, last.handle) != 0 {
		last.handle = 0
		filePublished = true
	}
	return nil
}

//go:uintptrescapes
func kernelCall(name string, args ...uintptr) uintptr {
	value, _, _ := capKernel.NewProc(name).Call(args...)
	return value
}
