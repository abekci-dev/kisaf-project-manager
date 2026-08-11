//go:build windows

// Package proc holds the few platform specific process-spawning knobs the rest
// of the code needs.
package proc

import (
	"os/exec"
	"syscall"
)

const (
	createNoWindow      = 0x08000000
	detachedProcess     = 0x00000008
	createNewProcessGrp = 0x00000200
	createNewConsole    = 0x00000010
)

// Hide runs a helper process with no console window. Without this every single
// `git` call would flash a black box on screen, dozens of times a minute.
func Hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// Detach launches a GUI program that must outlive us — closing kisaf should
// never take the user's editor down with it.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGrp}
}

// Console launches a program that needs its own visible console window
// (a terminal, a shell).
func Console(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole | createNewProcessGrp}
}
