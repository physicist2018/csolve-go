//go:build windows

package gui

import (
	"os/exec"
	"syscall"
)

// hideWindow прячет окно консоли дочернего процесса решателя на Windows,
// чтобы не появлялось лишнее окно при запуске GUI.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
