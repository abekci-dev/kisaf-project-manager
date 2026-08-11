// Package server exposes the HTTP API and serves the embedded web UI.
//
// # Threat model
//
// This process can open programs and read folders on the user's machine, so a
// stray web page must never be able to drive it. Three layers stop that:
//
//  1. Host allow-list — blocks DNS rebinding, where evil.com is made to resolve
//     to 127.0.0.1 so a browser will happily talk to us with the attacker's
//     origin.
//  2. Origin check on every mutating request — blocks plain CSRF from any page
//     the user happens to have open.
//  3. Token for anything that is not loopback — this is what makes exposing the
//     port to the LAN (or a homelab reverse proxy) safe.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
	"github.com/abekci-dev/kisaf-project-manager/internal/config"
	"github.com/abekci-dev/kisaf-project-manager/internal/gitx"
	"github.com/abekci-dev/kisaf-project-manager/internal/icon"
	"github.com/abekci-dev/kisaf-project-manager/internal/launcher"
	"github.com/abekci-dev/kisaf-project-manager/internal/store"
)

const tokenCookie = "kisaf_token"

// Server wires the pieces together and implements http.Handler.
type Server struct {
	cfg      config.Config
	store    *store.Store
	git      *gitx.Reader
	launcher *launcher.Launcher
	web      fs.FS
	assets   map[string]*asset
	handler  http.Handler

	mu        sync.RWMutex
	port      int
	startedAt time.Time
	hostname  string
	logf      func(string, ...any)
	onQuit    func()
}

// SetOnQuit registers the shutdown hook used by POST /api/quit, which is how a
// freshly launched build asks an older one to release the port.
func (s *Server) SetOnQuit(fn func()) {
	s.mu.Lock()
	s.onQuit = fn
	s.mu.Unlock()
}

// New builds a Server. web is the embedded UI filesystem.
func New(cfg config.Config, st *store.Store, web fs.FS, logf func(string, ...any)) *Server {
	if logf == nil {
		logf = log.Printf
	}
	host, _ := os.Hostname()
	s := &Server{
		cfg:       cfg,
		store:     st,
		git:       gitx.NewReader(),
		launcher:  launcher.New(),
		web:       web,
		startedAt: time.Now(),
		hostname:  strings.ToLower(host),
		logf:      logf,
	}
	s.assets = loadAssets(web, Version)
	s.handler = s.hostGuard(s.originGuard(s.auth(s.routes())))
	return s
}

// SetPort records the port actually bound, which the UI shows to the user.
func (s *Server) SetPort(p int) {
	s.mu.Lock()
	s.port = p
	s.mu.Unlock()
}

// Port returns the bound port.
func (s *Server) Port() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/quit", s.handleQuit)
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/editors", s.handleEditors)
	mux.HandleFunc("PUT /api/settings", s.handleSaveSettings)

	mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("GET /api/git/summary", s.handleGitSummary)
	mux.HandleFunc("GET /api/projects/{id}/git", s.handleGit)
	mux.HandleFunc("GET /api/projects/{id}/readme", s.handleReadme)
	mux.HandleFunc("GET /api/projects/{id}/tree", s.handleTree)
	mux.HandleFunc("GET /api/projects/{id}/file", s.handleFile)
	mux.HandleFunc("GET /api/projects/{id}/size", s.handleSize)
	mux.HandleFunc("POST /api/projects/{id}/open", s.handleOpen)

	mux.HandleFunc("POST /api/projects/{id}/todos", s.handleAddTodo)
	mux.HandleFunc("PATCH /api/projects/{id}/todos/{todoId}", s.handleUpdateTodo)
	mux.HandleFunc("DELETE /api/projects/{id}/todos/{todoId}", s.handleDeleteTodo)
	mux.HandleFunc("POST /api/projects/{id}/todos/clear-done", s.handleClearDoneTodos)
	mux.HandleFunc("PUT /api/projects/{id}/todos/order", s.handleReorderTodos)

	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("POST /api/import", s.handleImport)
	mux.HandleFunc("GET /api/fs", s.handleBrowse)

	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLoginSubmit)

	// Icons are drawn on demand rather than shipped as files; see internal/icon.
	mux.HandleFunc("GET /icons/", s.handleIcon)

	mux.Handle("/", s.staticHandler())
	return mux
}

// iconSpecs maps the paths referenced by index.html and the PWA manifest onto
// the drawing parameters for each image.
var iconSpecs = map[string]struct {
	size  int
	inset float64
}{
	"favicon-32.png":        {32, 0},
	"icon-192.png":          {192, 0},
	"icon-512.png":          {512, 0},
	"icon-maskable-512.png": {512, 0.14},
}

func (s *Server) handleIcon(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/icons/")

	var (
		data  []byte
		ctype string
		err   error
	)
	if name == "kisaf.ico" {
		data, ctype, err = nil, "image/x-icon", nil
		data, err = icon.ICO()
	} else if spec, ok := iconSpecs[name]; ok {
		ctype = "image/png"
		data, err = icon.PNG(spec.size, spec.inset)
	} else {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The drawing is deterministic per build, so the version is a sufficient
	// validator — and it changes when the artwork does.
	serveAsset(w, r, ctype, `"icon-`+Version+`-`+name+`"`, data)
}

// handleQuit lets a newly started build ask this one to release the port.
//
// It is loopback-only and needs a header no cross-site form can set, so a stray
// page cannot shut the app down; the request that matters comes from our own
// binary, not from a browser.
func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.RemoteAddr) {
		writeErrorCode(w, http.StatusForbidden, apperr.CodeQuitLocalOnly, "local requests only")
		return
	}
	if r.Header.Get("X-Kisaf-Quit") != "1" {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeQuitBadRequest, "missing header")
		return
	}

	s.mu.RLock()
	quit := s.onQuit
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": Version})

	if quit != nil {
		// Answer first, then stop: the caller is waiting for the port to free.
		go func() {
			time.Sleep(150 * time.Millisecond)
			quit()
		}()
	}
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// hostGuard rejects requests whose Host header is not a name we recognise.
//
// Without this, an attacker page on evil.com whose DNS points at 127.0.0.1
// would reach us with full same-origin privileges.
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "" {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(strings.Trim(host, "[]"))

		if !s.hostAllowed(host) {
			http.Error(w, "This host name is not allowed: "+host+
				"\nAdd it to allowedHosts in config.json if you need it.", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", s.cfg.Host, s.cfg.MDNSName(), s.hostname, s.hostname + ".local":
		return true
	}
	// A bare IP literal cannot be pointed at us by an attacker's DNS record.
	if net.ParseIP(host) != nil {
		return true
	}
	for _, allowed := range s.cfg.AllowedHosts {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return false
}

// originGuard blocks cross-site writes. Browsers always attach Origin to
// fetch()-issued POST/PATCH/DELETE, so a missing Origin means the request did
// not come from a page — curl, a script, the tray — and is allowed through.
func (s *Server) originGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		if !s.originMatchesHost(origin, r.Host) {
			writeErrorCode(w, http.StatusForbidden, apperr.CodeOriginRejected, "request from a different origin was rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originMatchesHost(origin, host string) bool {
	origin = strings.TrimSpace(strings.ToLower(origin))
	for _, prefix := range []string{"http://", "https://"} {
		origin = strings.TrimPrefix(origin, prefix)
	}
	return strings.EqualFold(origin, strings.ToLower(host))
}

// auth lets this machine through untouched and demands a token from everyone
// else. Requiring a token locally would only train the user to paste secrets
// into a tool that already runs with their own privileges.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopback(r.RemoteAddr) || r.URL.Path == "/login" {
			next.ServeHTTP(w, r)
			return
		}

		if s.cfg.Token == "" {
			msg := "Remote access is disabled. To enable it, set a passphrase in the \"token\" field of " + s.cfg.Path + " and restart."
			if isAPI(r) {
				writeErrorCode(w, http.StatusForbidden, apperr.CodeRemoteDisabled, "%s", msg)
			} else {
				http.Error(w, msg, http.StatusForbidden)
			}
			return
		}

		if s.tokenOK(r) {
			next.ServeHTTP(w, r)
			return
		}
		if isAPI(r) {
			writeErrorCode(w, http.StatusUnauthorized, apperr.CodeAuthRequired, "sign-in required")
			return
		}
		http.Redirect(w, r, "/login?next="+r.URL.EscapedPath(), http.StatusFound)
	})
}

func (s *Server) tokenOK(r *http.Request) bool {
	if c, err := r.Cookie(tokenCookie); err == nil && constantEqual(c.Value, s.cfg.Token) {
		return true
	}
	if h := r.Header.Get("X-kisaf-Token"); h != "" && constantEqual(h, s.cfg.Token) {
		return true
	}
	return false
}

func isAPI(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/api/") }

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// ---------------------------------------------------------------------------
// Static assets
// ---------------------------------------------------------------------------

// asset is one embedded UI file, hashed once at startup.
type asset struct {
	data  []byte
	etag  string
	ctype string
}

// loadAssets reads the embedded UI into memory and fingerprints each file.
//
// Serving from a map rather than http.FileServer is what makes upgrades work:
// embedded files have a zero modification time, so the standard file server
// cannot produce a usable Last-Modified or ETag, and the only cache policy left
// is a time-based guess. A content hash lets the browser revalidate cheaply and
// pick up a new build the moment the binary is replaced.
func loadAssets(web fs.FS, version string) map[string]*asset {
	out := map[string]*asset{}

	err := fs.WalkDir(web, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(web, path)
		if err != nil {
			return err
		}
		// The service worker names its cache after the build, so upgrading the
		// binary retires the previous cache instead of serving it forever.
		if path == "sw.js" {
			data = []byte(strings.ReplaceAll(string(data), "__KISAF_VERSION__", version))
		}

		sum := sha256.Sum256(data)
		ctype := mime.TypeByExtension(filepath.Ext(path))
		if ctype == "" {
			ctype = http.DetectContentType(data)
		}
		out[path] = &asset{
			data:  data,
			etag:  `"` + hex.EncodeToString(sum[:12]) + `"`,
			ctype: ctype,
		}
		return nil
	})
	if err != nil {
		log.Printf("could not read web assets: %v", err)
	}
	return out
}

func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		a, ok := s.assets[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Referrer-Policy", "same-origin")
		serveAsset(w, r, a.ctype, a.etag, a.data)
	})
}

// serveAsset answers with a validator instead of an expiry. Over loopback a
// revalidation round trip is far cheaper than the cost of one stale upgrade,
// and a 304 carries no body.
func serveAsset(w http.ResponseWriter, r *http.Request, ctype, etag string, data []byte) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(data); err != nil {
		log.Printf("could not send file: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("could not write response: %v", err)
	}
}

// writeError sends a failure the UI can localise.
//
// The body carries three things: the English text (what curl, logs and any
// untranslated UI show), a stable code, and the substitution arguments. With
// the code and the args the browser can rebuild the sentence in its own
// language instead of pattern-matching on English prose.
func writeError(w http.ResponseWriter, status int, err error) {
	body := map[string]any{"error": err.Error()}

	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		body["code"] = appErr.Code
		if len(appErr.Args) > 0 {
			body["args"] = appErr.Args
		}
	}
	writeJSON(w, status, body)
}

// writeErrorCode is the shorthand for failures raised inside the HTTP layer,
// which have no error value to carry around.
func writeErrorCode(w http.ResponseWriter, status int, code, format string, args ...any) {
	writeError(w, status, apperr.New(code, format, args...))
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20))
	return dec.Decode(dst)
}

// constantEqual compares two secrets without leaking their contents through
// timing. Lengths differ often enough that the early return is fine.
func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
