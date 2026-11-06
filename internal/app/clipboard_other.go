//go:build !windows

package app

import "github.com/egoist/mygo"

func platformWriteClipboardImage(_ uintptr, data []byte) error {
	return mygo.Clipboard.WriteImage(data)
}
