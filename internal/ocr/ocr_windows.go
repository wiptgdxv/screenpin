//go:build windows

package ocr

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"screenpin/internal/process"

	xdraw "golang.org/x/image/draw"
)

//go:embed recognize_ocr.ps1
var ocrScript string

func recognizeWindowsText(img image.Image, language string) (string, error) {
	dir, err := os.MkdirTemp("", "screenpin-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	tiles, err := prepareOCRImages(dir, img)
	if err != nil {
		return "", err
	}
	manifest, err := json.Marshal(tiles)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "tiles.json")
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		return "", err
	}
	wide := utf16.Encode([]rune(ocrScript))
	data := make([]byte, len(wide)*2)
	for i, v := range wide {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	process.HideWindow(cmd)
	cmd.Env = append(os.Environ(), "SCREENPIN_OCR_MANIFEST="+path, "SCREENPIN_OCR_LANGUAGE="+language)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return "", fmt.Errorf("文字识别超时，请缩小图片范围后重试")
	}
	switch string(output) {
	case "!language":
		return "", fmt.Errorf("未安装 OCR 语言，请在 Windows 的语言设置里安装所需语言的 OCR 功能")
	}
	if err != nil {
		return "", fmt.Errorf("Windows 文字识别失败：%w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

type ocrTile struct {
	File     string
	KeepFrom int
	KeepTo   int
}

func prepareOCRImages(dir string, img image.Image) ([]ocrTile, error) {
	bounds := img.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("图片为空")
	}
	const tileHeight = 1600
	const overlap = 96
	long := bounds.Dy() > tileHeight && bounds.Dy()/2 > bounds.Dx()
	var tiles []ocrTile
	for start := bounds.Min.Y; start < bounds.Max.Y; {
		end := bounds.Max.Y
		sample := bounds
		if long {
			end = min(start+tileHeight, bounds.Max.Y)
			sample.Min.Y = max(bounds.Min.Y, start-overlap)
			sample.Max.Y = min(bounds.Max.Y, end+overlap)
		}
		largest := max(sample.Dx(), sample.Dy())
		scale := min(2, float64(4096)/float64(largest))
		prepared := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(sample.Dx())*scale)), max(1, int(float64(sample.Dy())*scale))))
		draw.Draw(prepared, prepared.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		xdraw.CatmullRom.Scale(prepared, prepared.Bounds(), img, sample, draw.Over, nil)
		file := filepath.Join(dir, "tile-"+strconv.Itoa(len(tiles))+".png")
		if err := writeOCRImage(file, prepared); err != nil {
			return nil, err
		}
		tile := ocrTile{File: file}
		if long {
			tile.KeepFrom = (start - sample.Min.Y) * prepared.Bounds().Dy() / sample.Dy()
			tile.KeepTo = (end - sample.Min.Y) * prepared.Bounds().Dy() / sample.Dy()
		}
		tiles = append(tiles, tile)
		start = end
	}
	return tiles, nil
}

func writeOCRImage(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}
