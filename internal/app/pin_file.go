package app

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

func (a *app) pinClipboardFile() bool {
	paths, err := mygo.Clipboard.ReadFiles()
	if err != nil {
		return false
	}
	for _, path := range paths {
		img := readPinFile(path)
		if img == nil {
			continue
		}
		var data bytes.Buffer
		if png.Encode(&data, img) == nil {
			a.pinAsset(ui.NewBitmap(img).Pixelated(), data.Bytes(), filepath.Base(path), true)
			return true
		}
	}
	return false
}

func readPinFile(path string) image.Image {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > (64*1024*1024)/config.Height {
		return nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	img, _, _ := image.Decode(file)
	return img
}
