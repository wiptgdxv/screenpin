package ocr

import "image"

const (
	WeChat  = "wechat"
	Windows = "windows"
)

func RecognizeText(img image.Image, engine, language string) (text, actualEngine string, err error) {
	if engine == WeChat || (engine == "" && language == "") {
		text, err = recognizeWeChatText(img)
		if engine == WeChat || (err == nil && text != "") {
			return text, "微信 OCR", err
		}
	}
	text, err = recognizeWindowsText(img, language)
	return text, "Windows OCR", err
}
