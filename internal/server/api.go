package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
	"github.com/abekci-dev/kisaf-project-manager/internal/gitx"
	"github.com/abekci-dev/kisaf-project-manager/internal/launcher"
	"github.com/abekci-dev/kisaf-project-manager/internal/netdisc"
	"github.com/abekci-dev/kisaf-project-manager/internal/scan"
	"github.com/abekci-dev/kisaf-project-manager/internal/store"
)

// Version is stamped at build time via -ldflags.
var Version = "dev"

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": Version,
		"uptime":  time.Since(s.startedAt).Round(time.Second).String(),
	})
}

type serverInfo struct {
	Version     string    `json:"version"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	LocalURL    string    `json:"localUrl"`
	MDNSURL     string    `json:"mdnsUrl"`
	LANURLs     []string  `json:"lanUrls"`
	DataFile    string    `json:"dataFile"`
	ConfigFile  string    `json:"configFile"`
	GitOK       bool      `json:"gitOk"`
	RemoteOpen  bool      `json:"remoteOpen"`
	OS          string    `json:"os"`
	StartedAt   time.Time `json:"startedAt"`
	MDNSEnabled bool      `json:"mdnsEnabled"`
}

// handleState is the single call the UI makes on load: projects, settings,
// editors, tags and server info in one round trip.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	port := s.Port()
	lan := []string{}
	for _, ip := range netdisc.LocalIPs() {
		lan = append(lan, netdisc.URLFor(ip, port))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"projects": s.store.Projects(),
		"settings": s.store.Settings(),
		"tags":     s.store.Tags(),
		"editors":  s.allEditors(false),
		"server": serverInfo{
			Version:     Version,
			Host:        s.cfg.Host,
			Port:        port,
			LocalURL:    netdisc.URLFor("localhost", port),
			MDNSURL:     netdisc.URLFor(s.cfg.MDNSName(), port),
			LANURLs:     lan,
			DataFile:    s.store.Path(),
			ConfigFile:  s.cfg.Path,
			GitOK:       gitx.Available(),
			RemoteOpen:  s.cfg.Token != "",
			OS:          osName(),
			StartedAt:   s.startedAt,
			MDNSEnabled: s.cfg.EnableMDNS,
		},
	})
}

// allEditors merges auto-detected launchers with the user's custom entries.
func (s *Server) allEditors(force bool) []launcher.Editor {
	editors := s.launcher.Editors(force)
	for _, ce := range s.store.Settings().CustomEditors {
		if strings.TrimSpace(ce.Exec) == "" {
			continue
		}
		var args []string
		if strings.TrimSpace(ce.Args) != "" {
			args = strings.Fields(ce.Args)
		}
		editors = append(editors, launcher.Editor{
			ID:   ce.ID,
			Name: ce.Name,
			Exec: ce.Exec,
			Args: args,
			Kind: "custom",
		})
	}
	return editors
}

func (s *Server) handleEditors(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("refresh") == "1"
	writeJSON(w, http.StatusOK, map[string]any{"editors": s.allEditors(force)})
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	out, err := s.store.SaveSettings(in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": out})
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in store.Project
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	p, err := s.store.Add(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": p})
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var patch store.Patch
	if err := decodeJSON(r, &patch); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	p, err := s.store.Update(r.PathValue("id"), patch)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	s.git.Invalidate(p.Path)
	writeJSON(w, http.StatusOK, map[string]any{"project": p})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.PathValue("id")); err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGit(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	limit := s.store.Settings().CommitCount
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if r.URL.Query().Get("refresh") == "1" {
		s.git.Invalidate(p.Path)
	}
	writeJSON(w, http.StatusOK, map[string]any{"git": s.git.Read(p.Path, limit)})
}

// handleGitSummary returns the branch/dirty badge data for every project in one
// request. The work is spread over a small pool so a list of 80 repositories
// does not turn into 80 sequential process spawns.
func (s *Server) handleGitSummary(w http.ResponseWriter, r *http.Request) {
	projects := s.store.Projects()

	type result struct {
		id  string
		sum gitx.Summary
	}
	results := make(chan result, len(projects))
	jobs := make(chan store.Project)

	workers := 8
	if len(projects) < workers {
		workers = len(projects)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				results <- result{id: p.ID, sum: gitx.ReadSummary(p.Path)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, p := range projects {
			select {
			case jobs <- p:
			case <-r.Context().Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	out := map[string]gitx.Summary{}
	for res := range results {
		out[res.id] = res.sum
	}
	writeJSON(w, http.StatusOK, map[string]any{"summaries": out})
}

const maxReadmeBytes = 512 << 10

func (s *Server) handleReadme(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, apperr.CodeDirUnreadable, "cannot read folder: %v", err)
		return
	}
	name := ""
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if lower == "readme.md" || lower == "readme.markdown" {
			name = e.Name()
			break
		}
		if name == "" && (lower == "readme" || lower == "readme.txt") {
			name = e.Name()
		}
	}
	if name == "" {
		writeJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	content, truncated, err := readTextFile(filepath.Join(p.Path, name), maxReadmeBytes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"found": true, "name": name, "content": content, "truncated": truncated,
	})
}

type treeEntry struct {
	Name    string    `json:"name"`
	Rel     string    `json:"rel"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// handleTree lists a single directory level. Lazy expansion keeps a repository
// with 40k files from ever being enumerated in one go.
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("rel")
	dir, err := safeJoin(p.Path, rel)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, apperr.CodeDirUnreadable, "cannot read folder: %v", err)
		return
	}

	out := make([]treeEntry, 0, len(entries))
	for _, e := range entries {
		if len(out) >= 1000 {
			break
		}
		item := treeEntry{
			Name:  e.Name(),
			Rel:   path.Join(rel, e.Name()),
			IsDir: e.IsDir(),
		}
		if info, err := e.Info(); err == nil {
			item.Size = info.Size()
			item.ModTime = info.ModTime()
		}
		out = append(out, item)
	}
	// Folders first, then files, both alphabetical — the order every file
	// manager uses.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeJSON(w, http.StatusOK, map[string]any{"rel": rel, "entries": out})
}

const maxFileBytes = 256 << 10

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	target, err := safeJoin(p.Path, r.URL.Query().Get("rel"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		writeErrorCode(w, http.StatusNotFound, apperr.CodeFileNotFound, "file not found")
		return
	}
	content, truncated, err := readTextFile(target, maxFileBytes)
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": filepath.Base(target), "content": content,
		"truncated": truncated, "size": info.Size(),
	})
}

// handleSize walks the project folder. It is deliberately a separate,
// on-demand call: doing this for every project on page load would hammer the
// disk for information the user rarely needs.
func (s *Server) handleSize(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var total int64
	var files int64
	err := filepath.WalkDir(p.Path, func(_ string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
			files++
		}
		return nil
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"bytes": total, "files": files, "partial": err != nil,
	})
}

type openRequest struct {
	Action string `json:"action"` // editor | reveal | folder | terminal
	Editor string `json:"editor"`
}

func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	p, ok := s.project(w, r)
	if !ok {
		return
	}
	var in openRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}

	req := launcher.OpenRequest{Path: p.Path, Action: in.Action, Prefer: s.store.Settings().TerminalPrefer}
	if in.Action == "editor" {
		id := firstNonEmpty(in.Editor, p.Editor, s.store.Settings().DefaultEditor)
		editor, found := s.findEditor(id)
		if !found {
			writeErrorCode(w, http.StatusBadRequest, apperr.CodeEditorNotFound,
				"No editor found. Pick a default editor in settings, or press \"rescan\".")
			return
		}
		req.Editor = editor
	}

	if err := s.launcher.Open(req); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if in.Action == "editor" {
		s.store.MarkOpened(p.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// findEditor resolves an id against the detected + custom list. Only ids that
// survive this lookup can ever be executed.
func (s *Server) findEditor(id string) (launcher.Editor, bool) {
	editors := s.allEditors(false)
	if id != "" {
		for _, e := range editors {
			if e.ID == id {
				return e, true
			}
		}
	}
	// No preference set yet: fall back to the first editor we know about so
	// the very first click still does something useful.
	if len(editors) > 0 {
		return editors[0], true
	}
	return launcher.Editor{}, false
}

// ---------------------------------------------------------------------------
// Scan & import
// ---------------------------------------------------------------------------

type scanRequest struct {
	Root          string `json:"root"`
	Depth         int    `json:"depth"`
	IncludeNonGit bool   `json:"includeNonGit"`
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var in scanRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}
	root, err := store.NormalizePath(in.Root)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	results, err := scan.Run(ctx, scan.Options{
		Root:          root,
		Depth:         in.Depth,
		IncludeNonGit: in.IncludeNonGit,
		Known:         s.store.KnownPaths(),
	})
	if err != nil && len(results) == 0 {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeScanFailed, "scan failed: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": root, "results": results})
}

type importRequest struct {
	Paths []string `json:"paths"`
	Tags  []string `json:"tags"`
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	var in importRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErrorCode(w, http.StatusBadRequest, apperr.CodeRequestInvalid, "invalid request: %v", err)
		return
	}

	added := []store.Project{}
	skipped := []string{}
	for _, p := range in.Paths {
		project, err := s.store.Add(store.Project{Path: p, Tags: in.Tags})
		if err != nil {
			skipped = append(skipped, filepath.Base(p)+": "+err.Error())
			continue
		}
		added = append(added, project)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"added": added, "addedCount": len(added), "skipped": skipped,
	})
}

// ---------------------------------------------------------------------------
// Folder picker
// ---------------------------------------------------------------------------

type browseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsGit bool   `json:"isGit"`
}

// handleBrowse powers the folder picker in the UI. A browser cannot show a
// native directory dialog that yields a real path, so the server lists folders
// instead.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")

	if strings.TrimSpace(raw) == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"path": "", "parent": "", "roots": rootFolders(), "entries": []browseEntry{},
		})
		return
	}

	dir, err := store.NormalizePath(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, apperr.CodeDirUnreadable, "cannot open folder: %v", err)
		return
	}

	out := make([]browseEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || len(out) >= 2000 {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") && e.Name() != ".config" {
			continue
		}
		full := filepath.Join(dir, e.Name())
		item := browseEntry{Name: e.Name(), Path: full}
		if _, err := os.Stat(filepath.Join(full, ".git")); err == nil {
			item.IsGit = true
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	parent := filepath.Dir(dir)
	if parent == dir {
		parent = "" // already at a drive root / filesystem root
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": dir, "parent": parent, "roots": rootFolders(), "entries": out,
	})
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (s *Server) project(w http.ResponseWriter, r *http.Request) (store.Project, bool) {
	p, err := s.store.Get(r.PathValue("id"))
	if err != nil {
		s.writeStoreError(w, err)
		return store.Project{}, false
	}
	return p, true
}

func (s *Server) writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusInternalServerError, err)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
