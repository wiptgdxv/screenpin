//go:build !windows

package app

import "errors"

func startSystemAudio() ([]string, func(), error) {
	return nil, nil, errors.New("系统声音录制仅支持 Windows")
}
