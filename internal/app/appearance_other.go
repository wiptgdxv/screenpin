//go:build !windows

package app

func clearCompositionSupported() bool { return false }
