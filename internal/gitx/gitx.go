// Package gitx reads repository state by shelling out to the git binary.
//
// Shelling out beats embedding a git implementation here: it is a fraction of
// the code, it always agrees with what the user sees in their own terminal, and
// it costs nothing when a folder is not a repository.
package gitx

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abekci-dev/kisaf-project-manager/internal/proc"
)

// Commit is a single entry from `git log`.
type Commit struct {
	Hash    string    `json:"hash"`
	Short   string    `json:"short"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	Author  string    `json:"author"`
	Email   string    `json:"email"`
	Date    time.Time `json:"date"`
	Refs    string    `json:"refs"`
}

// Info is everything the UI shows about a repository.
type Info struct {
	IsRepo     bool     `json:"isRepo"`
	Available  bool     `json:"available"` // is the git binary installed at all
	Branch     string   `json:"branch"`
	Upstream   string   `json:"upstream"`
	RemoteURL  string   `json:"remoteUrl"`
	Ahead      int      `json:"ahead"`
	Behind     int      `json:"behind"`
	Staged     int      `json:"staged"`
	Unstaged   int      `json:"unstaged"`
	Untracked  int      `json:"untracked"`
	Conflicted int      `json:"conflicted"`
	Dirty      bool     `json:"dirty"`
	Branches   []string `json:"branches"`
	Commits    []Commit `json:"commits"`
	Changes    []Change `json:"changes"`
	Error      string   `json:"error,omitempty"`
}

// Change is one line of `git status --porcelain`.
type Change struct {
	Status string `json:"status"`
	Path   string `json:"path"`
	Kind   string `json:"kind"` // staged | unstaged | untracked | conflict
}

// unit separator / record separator, chosen because they cannot appear in a
// commit subject or author name.
const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

type cacheEntry struct {
	info Info
	at   time.Time
	n    int
}

// Reader caches results briefly so that clicking through the project list does
// not spawn a burst of git processes for the same folder.
type Reader struct {
	mu    sync.Mutex
	cache map[string]cacheEntry
	ttl   time.Duration
}

// NewReader builds a Reader with a short cache window.
func NewReader() *Reader {
	return &Reader{cache: map[string]cacheEntry{}, ttl: 4 * time.Second}
}

// Available reports whether a git binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// Read returns repository information for dir, including the newest n commits.
func (r *Reader) Read(dir string, n int) Info {
	if n <= 0 {
		n = 25
	}
	r.mu.Lock()
	if e, ok := r.cache[dir]; ok && e.n >= n && time.Since(e.at) < r.ttl {
		r.mu.Unlock()
		return e.info
	}
	r.mu.Unlock()

	info := read(dir, n)

	r.mu.Lock()
	r.cache[dir] = cacheEntry{info: info, at: time.Now(), n: n}
	r.mu.Unlock()
	return info
}

// Invalidate drops the cached entry for dir.
func (r *Reader) Invalidate(dir string) {
	r.mu.Lock()
	delete(r.cache, dir)
	r.mu.Unlock()
}

// Summary is the small slice of repository state the project list shows.
type Summary struct {
	IsRepo  bool   `json:"isRepo"`
	Branch  string `json:"branch"`
	Dirty   bool   `json:"dirty"`
	Changes int    `json:"changes"`
	Ahead   int    `json:"ahead"`
	Behind  int    `json:"behind"`
}

type summaryEntry struct {
	value Summary
	at    time.Time
}

var (
	summaryMu    sync.Mutex
	summaryCache = map[string]summaryEntry{}
)

// ReadSummary is the cheap sibling of Read: two git calls instead of six, and
// no commit history. The project list needs this for every row at once, so the
// difference decides whether the sidebar renders instantly or crawls.
func ReadSummary(dir string) Summary {
	summaryMu.Lock()
	if e, ok := summaryCache[dir]; ok && time.Since(e.at) < 10*time.Second {
		summaryMu.Unlock()
		return e.value
	}
	summaryMu.Unlock()

	var out Summary
	if Available() {
		if o, err := run(dir, "status", "--porcelain=v1", "--branch", "--untracked-files=normal"); err == nil {
			var info Info
			parseStatus(&info, o)
			out = Summary{
				IsRepo:  true,
				Branch:  info.Branch,
				Dirty:   info.Dirty,
				Changes: info.Staged + info.Unstaged + info.Untracked + info.Conflicted,
				Ahead:   info.Ahead,
				Behind:  info.Behind,
			}
		}
	}

	summaryMu.Lock()
	summaryCache[dir] = summaryEntry{value: out, at: time.Now()}
	summaryMu.Unlock()
	return out
}

func read(dir string, n int) Info {
	info := Info{Available: Available()}
	if !info.Available {
		info.Error = "git command not found (is it on PATH?)"
		return info
	}

	if out, err := run(dir, "rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return info
	}
	info.IsRepo = true

	if out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		info.Branch = strings.TrimSpace(out)
		if info.Branch == "HEAD" {
			// Detached head — show the short hash instead, it is more useful.
			if short, err := run(dir, "rev-parse", "--short", "HEAD"); err == nil {
				info.Branch = "detached @ " + strings.TrimSpace(short)
			}
		}
	}
	if out, err := run(dir, "remote", "get-url", "origin"); err == nil {
		info.RemoteURL = strings.TrimSpace(out)
	}
	if out, err := run(dir, "status", "--porcelain=v1", "--branch", "--untracked-files=normal"); err == nil {
		parseStatus(&info, out)
	}
	if out, err := run(dir, "branch", "--format=%(refname:short)"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				info.Branches = append(info.Branches, line)
			}
		}
	}

	format := strings.Join([]string{"%H", "%h", "%s", "%an", "%ae", "%aI", "%D", "%b"}, fieldSep) + recordSep
	if out, err := run(dir, "log", "-n", strconv.Itoa(n), "--pretty=format:"+format); err == nil {
		info.Commits = parseLog(out)
	} else {
		// An empty repository has no HEAD yet; that is not an error worth showing.
		info.Commits = []Commit{}
	}

	if info.Commits == nil {
		info.Commits = []Commit{}
	}
	if info.Branches == nil {
		info.Branches = []string{}
	}
	if info.Changes == nil {
		info.Changes = []Change{}
	}
	return info
}

// parseStatus reads `git status --porcelain=v1 --branch` output. The first line
// looks like "## main...origin/main [ahead 1, behind 2]".
func parseStatus(info *Info, out string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			parseBranchLine(info, strings.TrimPrefix(line, "## "))
			continue
		}
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		path := line[3:]
		// Renames read as "old -> new"; the destination is what the user cares about.
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		change := Change{Status: line[:2], Path: strings.Trim(path, "\"")}
		switch {
		case x == '?' && y == '?':
			change.Kind = "untracked"
			info.Untracked++
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			change.Kind = "conflict"
			info.Conflicted++
		default:
			if x != ' ' {
				info.Staged++
			}
			if y != ' ' {
				info.Unstaged++
			}
			if x != ' ' {
				change.Kind = "staged"
			} else {
				change.Kind = "unstaged"
			}
		}
		if len(info.Changes) < 300 {
			info.Changes = append(info.Changes, change)
		}
	}
	info.Dirty = info.Staged+info.Unstaged+info.Untracked+info.Conflicted > 0
}

func parseBranchLine(info *Info, line string) {
	tracking := ""
	if idx := strings.Index(line, " ["); idx >= 0 {
		tracking = strings.Trim(line[idx+2:], "[]")
		line = line[:idx]
	}
	if idx := strings.Index(line, "..."); idx >= 0 {
		info.Upstream = line[idx+3:]
		line = line[:idx]
	}
	if info.Branch == "" {
		info.Branch = line
	}
	for _, part := range strings.Split(tracking, ", ") {
		fields := strings.Fields(part)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			info.Ahead = n
		case "behind":
			info.Behind = n
		}
	}
}

func parseLog(out string) []Commit {
	commits := []Commit{}
	for _, record := range strings.Split(out, recordSep) {
		record = strings.TrimLeft(record, "\r\n")
		if strings.TrimSpace(record) == "" {
			continue
		}
		parts := strings.Split(record, fieldSep)
		if len(parts) < 7 {
			continue
		}
		c := Commit{
			Hash:    parts[0],
			Short:   parts[1],
			Subject: parts[2],
			Author:  parts[3],
			Email:   parts[4],
			Refs:    parts[6],
		}
		if len(parts) > 7 {
			c.Body = strings.TrimSpace(parts[7])
		}
		if t, err := time.Parse(time.RFC3339, parts[5]); err == nil {
			c.Date = t
		}
		commits = append(commits, c)
	}
	return commits
}

func run(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// Keep git from asking for credentials or opening a pager: either would
	// hang the request forever.
	cmd.Env = append(cmd.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GCM_INTERACTIVE=never",
	)
	proc.Hide(cmd)

	out, err := cmd.Output()
	return string(out), err
}
