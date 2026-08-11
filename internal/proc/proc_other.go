//go:build !windows

// Package proc holds the few platform specific process-spawning knobs the rest
// of the code needs.
package proc

import (
	"os/exec"
	"syscall"
)

// Hide is a no-op away from Windows: there is no console window to suppress.
func Hide(cmd *exec.Cmd) {}

// Detach puts the child in its own session so it survives our exit.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// Console launches a program that needs its own terminal window.
func Console(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
