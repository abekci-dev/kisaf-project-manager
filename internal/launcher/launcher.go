// Package launcher finds the editors installed on this machine and opens
// project folders with them, with the file manager, or with a terminal.
//
// Security note: the HTTP API never receives a command to run. It receives an
// editor *id*, which is resolved against this detected list (or the user's
// explicitly configured custom editors) before anything is executed. There is
// no path from a request body to an arbitrary process.
package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
)

// Editor is one launchable program.
type Editor struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Exec     string   `json:"exec"`
	Args     []string `json:"args"`
	Kind     string   `json:"kind"` // "editor" | "custom"
	Detected bool     `json:"detected"`
}

// Launcher caches editor detection, which touches the filesystem and is far
// too slow to redo on every page load.
type Launcher struct {
	mu         sync.Mutex
	cache      []Editor
	cachedAt   time.Time
	cacheTTL   time.Duration
	terminalID string
}

// New builds a Launcher.
func New() *Launcher {
	return &Launcher{cacheTTL: 5 * time.Minute}
}

// Editors returns every detected editor. Pass force to re-scan immediately,
// which is what the "yeniden tara" button in settings does.
func (l *Launcher) Editors(force bool) []Editor {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !force && l.cache != nil && time.Since(l.cachedAt) < l.cacheTTL {
		return append([]Editor(nil), l.cache...)
	}
	found := detectEditors()
	sort.SliceStable(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	l.cache = found
	l.cachedAt = time.Now()
	return append([]Editor(nil), found...)
}

// OpenRequest describes one launch.
type OpenRequest struct {
	Path   string
	Action string // editor | reveal | folder | terminal
	Editor Editor // only consulted when Action == "editor"
	Prefer string // preferred terminal id, only for Action == "terminal"
}

// Open performs the requested action.
func (l *Launcher) Open(req OpenRequest) error {
	info, err := os.Stat(req.Path)
	if err != nil {
		return apperr.Wrap(err, apperr.CodeProjectUnopened, "folder not found: %v", err)
	}
	if !info.IsDir() {
		return apperr.New(apperr.CodeProjectNotDir, "the project path is not a folder")
	}

	switch req.Action {
	case "editor":
		if strings.TrimSpace(req.Editor.Exec) == "" {
			return apperr.New(apperr.CodeEditorNotChosen, "no editor selected or found")
		}
		return runEditor(req.Editor, req.Path)
	case "reveal":
		return reveal(req.Path)
	case "folder":
		return openFolder(req.Path)
	case "terminal":
		return openTerminal(req.Path, req.Prefer)
	default:
		return apperr.New(apperr.CodeUnknownAction, "unknown action: %s", req.Action)
	}
}

// OpenURL asks the operating system to open a URL in the default browser.
func OpenURL(url string) error { return openURL(url) }

func runEditor(e Editor, path string) error {
	args := e.Args
	if len(args) == 0 {
		args = []string{"{path}"}
	}
	expanded := make([]string, 0, len(args))
	for _, a := range args {
		expanded = append(expanded, strings.ReplaceAll(a, "{path}", path))
	}
	cmd := buildDetached(e.Exec, expanded)
	cmd.Dir = path
	return start(cmd)
}

// start launches a process and immediately stops caring about it. Wait is still
// called in the background so we do not leave zombies behind on unix.
func start(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// firstExisting returns the first path that exists on disk, expanding globs.
func firstExisting(candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if strings.ContainsAny(c, "*?") {
			matches, _ := globSorted(c)
			if len(matches) > 0 {
				return matches[0]
			}
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// lookPath is exec.LookPath with the error swallowed.
func lookPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// globSorted expands a glob newest-name-first, so that a pattern like
// "JetBrains\*\bin\idea64.exe" prefers IntelliJ 2025.2 over 2023.1.
func globSorted(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return matches, nil
}
