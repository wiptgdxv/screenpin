//go:build !windows

package ocr

import (
	"fmt"
	"image"
)

func recognizeWeChatText(image.Image) (string, error) {
	return "", fmt.Errorf("微信 OCR 仅支持 Windows")
}
