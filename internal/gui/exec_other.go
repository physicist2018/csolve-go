//go:build !windows

package gui

import "os/exec"

// hideWindow — нет-op на не-Windows платформах.
func hideWindow(_ *exec.Cmd) {}
