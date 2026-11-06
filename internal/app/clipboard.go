package app

func (a *app) writeClipboardImage(data []byte) error {
	return platformWriteClipboardImage(a.window.NativeHandle(), data)
}
