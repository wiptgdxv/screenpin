//go:build !windows

package ocr

import (
	"fmt"
	"image"
)

func recognizeWindowsText(image.Image, string) (string, error) {
	return "", fmt.Errorf("当前文字识别仅支持 Windows")
}
