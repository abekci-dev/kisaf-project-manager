//go:build !windows

// Package tray provides a notification-area icon on Windows. On other systems
// there is no single tray API worth depending on, so the process simply runs in
// the foreground.
package tray

import (
	"log"
	"os"
)

// Options mirrors the Windows implementation so main.go stays platform free.
type Options struct {
	Title    string
	URL      string
	AltURL   string
	DataDir  string
	IconPath string
	OnQuit   func()
	Logf     func(string, ...any)
}

// Run reports that no tray is available; the caller then blocks on signals.
func Run(opts Options, signals <-chan os.Signal) bool { return false }

// Alert falls back to the log, which is where a headless run looks anyway.
func Alert(title, message string) { log.Printf("%s: %s", title, message) }
