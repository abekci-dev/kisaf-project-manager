// Command kisaf is a lightweight local project manager.
//
// It keeps track of the project folders on this machine, shows their git state,
// and opens them in your editor, file manager or terminal — from a web UI
// served by this same binary at http://kisaf.local (or http://kisaf on
// Windows), so there is no "localhost:1234" to remember.
//
// Copyright (C) 2026 the kisaf authors.
//
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version. This program is distributed WITHOUT ANY WARRANTY; without even the
// implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See
// the GNU General Public License for more details, in the LICENSE file at the
// root of this repository or at <https://www.gnu.org/licenses/>.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/config"
	"github.com/abekci-dev/kisaf-project-manager/internal/icon"
	"github.com/abekci-dev/kisaf-project-manager/internal/launcher"
	"github.com/abekci-dev/kisaf-project-manager/internal/netdisc"
	"github.com/abekci-dev/kisaf-project-manager/internal/server"
	"github.com/abekci-dev/kisaf-project-manager/internal/store"
	"github.com/abekci-dev/kisaf-project-manager/internal/tray"
)

//go:embed all:web
var webRoot embed.FS

// version is overridden at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	var (
		flagPort    = flag.Int("port", -1, "port to listen on (default: config.json)")
		flagNoTray  = flag.Bool("no-tray", false, "do not start the system tray icon")
		flagNoOpen  = flag.Bool("no-browser", false, "do not open a browser at startup")
		flagNoMDNS  = flag.Bool("no-mdns", false, "disable network discovery (the .local name)")
		flagVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Println("kisaf", version)
		return
	}

	server.Version = version

	cfg, err := config.Load()
	if err != nil {
		fatal("Could not read settings", err)
	}
	if *flagPort >= 0 {
		cfg.Port = *flagPort
	}
	if *flagNoTray {
		cfg.EnableTray = false
	}
	if *flagNoOpen {
		cfg.OpenBrowser = false
	}
	if *flagNoMDNS {
		cfg.EnableMDNS = false
	}

	closeLog := setupLogging(cfg.DataDir)
	defer closeLog()

	log.Printf("kisaf %s starting (data: %s)", version, cfg.DataDir)
	if cfg.MigratedFrom != "" {
		log.Printf("carried your project list over from the previous install at %s (the old folder was left untouched)", cfg.MigratedFrom)
	}

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		// A corrupt data file is reported but never fatal: the user should get
		// a working window back, not a binary that refuses to start.
		log.Printf("warning: %v", err)
		if st == nil {
			fatal("Could not open the project list", err)
		}
	}

	webFS, err := fs.Sub(webRoot, "web")
	if err != nil {
		fatal("Could not load the web interface", err)
	}

	srv := server.New(cfg, st, webFS, log.Printf)

	listener, port, err := listen(cfg)
	if err != nil {
		fatal("Could not start the server", err)
	}
	srv.SetPort(port)

	httpServer := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
		}
	}()

	var responder *netdisc.Responder
	if cfg.EnableMDNS {
		responder = netdisc.Start(cfg.Host, log.Printf)
	}

	localURL := netdisc.URLFor("localhost", port)
	niceURL := localURL
	if cfg.EnableMDNS {
		niceURL = netdisc.URLFor(cfg.MDNSName(), port)
	}
	printBanner(cfg, port, localURL)

	if cfg.OpenBrowser {
		// Open the loopback URL: it is the one that resolves instantly, with no
		// dependency on mDNS having propagated yet.
		if err := launcher.OpenURL(localURL); err != nil {
			log.Printf("could not open a browser: %v", err)
		}
	}

	shutdown := func() {
		log.Print("shutting down")
		if responder != nil {
			responder.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	// A newer build asking us to step aside is handled exactly like Ctrl+C, so
	// the tray's message loop unwinds the same way instead of being left behind.
	srv.SetOnQuit(func() {
		select {
		case signals <- syscall.SIGTERM:
		default:
		}
	})

	// On Windows the tray owns the main thread and its message loop; elsewhere
	// (and with --no-tray) we simply block until a signal arrives.
	ran := false
	if cfg.EnableTray {
		ran = tray.Run(tray.Options{
			Title:    "kisaf project manager",
			URL:      niceURL,
			AltURL:   localURL,
			DataDir:  cfg.DataDir,
			IconPath: writeTrayIcon(cfg.DataDir),
			OnQuit:   shutdown,
			Logf:     log.Printf,
		}, signals)
	}
	if !ran {
		<-signals
		shutdown()
	}
}

// listen binds the preferred port, falling back through the configured list.
//
// A busy port occupied by another kisaf is handled by version:
//   - same version  -> that copy is already what the user wants; bring it to
//     the front and exit rather than starting a second server
//   - older version -> ask it to quit and take the port over, so replacing the
//     binary actually replaces the running program. Without this, launching a
//     new build silently opens the old one, and the old .exe stays locked and
//     undeletable on Windows.
func listen(cfg config.Config) (net.Listener, int, error) {
	ports := append([]int{cfg.Port}, cfg.FallbackPorts...)
	var lastErr error

	for _, port := range ports {
		addr := net.JoinHostPort(cfg.Bind, fmt.Sprint(port))
		if ln, err := net.Listen("tcp", addr); err == nil {
			actual := ln.Addr().(*net.TCPAddr).Port
			if port != cfg.Port {
				log.Printf("port %d is busy, switched to %d", cfg.Port, actual)
			}
			return ln, actual, nil
		} else {
			lastErr = err
		}

		if port == 0 {
			continue
		}
		other, ok := probeInstance(port)
		if !ok {
			continue // some other program holds the port; try the next one
		}

		if other == version {
			url := netdisc.URLFor("localhost", port)
			log.Printf("kisaf (%s) is already running, opening the browser: %s", other, url)
			_ = launcher.OpenURL(url)
			os.Exit(0)
		}

		log.Printf("an older kisaf (%s) holds port %d; asking it to quit and taking over", other, port)
		if !requestQuit(port) {
			log.Printf("could not stop the old copy; end kisaf.exe from Task Manager")
			continue
		}
		if ln, err := waitForPort(cfg.Bind, port, 8*time.Second); err == nil {
			return ln, ln.Addr().(*net.TCPAddr).Port, nil
		} else {
			lastErr = err
		}
	}
	return nil, 0, lastErr
}

// healthResponse is the part of /api/health this process cares about.
type healthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

// probeInstance reports the version of the kisaf occupying a busy port.
func probeInstance(port int) (string, bool) {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var health healthResponse
	if err := json.Unmarshal(body, &health); err != nil || !health.OK {
		return "", false
	}
	return health.Version, true
}

// requestQuit asks the running copy to shut down.
func requestQuit(port int) bool {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/quit", port), nil)
	if err != nil {
		return false
	}
	req.Header.Set("X-Kisaf-Quit", "1")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}

// waitForPort polls until the old process has released the socket.
func waitForPort(bind string, port int, timeout time.Duration) (net.Listener, error) {
	addr := net.JoinHostPort(bind, fmt.Sprint(port))
	deadline := time.Now().Add(timeout)

	var lastErr error
	for time.Now().Before(deadline) {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return nil, lastErr
}

// setupLogging tees the log to a file so a GUI build (which has no console)
// still leaves a trail when something goes wrong.
func setupLogging(dir string) func() {
	path := filepath.Join(dir, "kisaf.log")

	// Keep the log from growing forever; there is nothing here worth archiving.
	if info, err := os.Stat(path); err == nil && info.Size() > 2<<20 {
		_ = os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.SetFlags(log.LstdFlags)
		return func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags)
	return func() { _ = f.Close() }
}

func printBanner(cfg config.Config, port int, localURL string) {
	lines := []string{
		"",
		"  kisaf project manager " + version,
		"  ─────────────────────────────────",
		"  This computer : " + localURL,
	}
	if cfg.EnableMDNS {
		lines = append(lines,
			"  On the LAN    : "+netdisc.URLFor(cfg.MDNSName(), port),
			"  On Windows    : "+netdisc.URLFor(cfg.Host, port),
		)
	}
	for _, ip := range netdisc.LocalIPs() {
		lines = append(lines, "  By IP         : "+netdisc.URLFor(ip, port))
	}
	lines = append(lines, "  Data folder   : "+cfg.DataDir, "")
	fmt.Println(strings.Join(lines, "\n"))
}

// writeTrayIcon renders the icon to a real file next to the data, because the
// Windows shell can only load a tray icon from a path — it has no way to take
// bytes we already hold in memory.
func writeTrayIcon(dir string) string {
	data, err := icon.ICO()
	if err != nil {
		log.Printf("could not draw the tray icon: %v", err)
		return ""
	}
	path := filepath.Join(dir, "kisaf.ico")
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(data)) {
		return path // same build already written
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("could not write the tray icon: %v", err)
		return ""
	}
	return path
}

// fatal reports a startup failure. On a GUI build there is no console to print
// to, so tray.Alert puts it in a message box instead.
func fatal(context string, err error) {
	msg := context + ": " + err.Error()
	log.Print(msg)
	tray.Alert("kisaf", msg)
	os.Exit(1)
}
