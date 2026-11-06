//go:build !windows

package process

import "os/exec"

func HideWindow(*exec.Cmd)            {}
func Guard(*exec.Cmd) (func(), error) { return func() {}, nil }
