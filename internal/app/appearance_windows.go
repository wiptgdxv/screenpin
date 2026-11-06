//go:build windows

package app

import "golang.org/x/sys/windows"

func clearCompositionSupported() bool { return windows.RtlGetVersion().BuildNumber >= 22621 }
