package ocr

import (
	"fmt"
	"image"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/multi/qrcode"
)

func DecodeQR(img image.Image) (string, error) {
	bitmap, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	result, err := qrcode.NewQRCodeMultiReader().DecodeMultiple(bitmap, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true})
	if err != nil {
		return "", fmt.Errorf("未识别到二维码。请保留二维码四周的空白边缘")
	}
	texts := make([]string, 0, len(result))
	for _, code := range result {
		texts = append(texts, code.GetText())
	}
	return strings.Join(texts, "\n\n"), nil
}
