// Package scan walks a root folder and reports the projects it finds.
//
// This is the answer to "too many folders to keep track of": point it at
// D:\Projeler once and every repository underneath shows up as an importable
// row instead of being typed in by hand.
package scan

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Result is one discovered project folder.
type Result struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	IsGit     bool      `json:"isGit"`
	Kinds     []string  `json:"kinds"`
	Modified  time.Time `json:"modified"`
	Known     bool      `json:"known"`
	Remote    string    `json:"remote"`
	HasReadme bool      `json:"hasReadme"`
}

// Options controls one scan.
type Options struct {
	Root string
	// Depth is how many folder levels below Root to inspect. 0 means "only
	// Root itself".
	Depth int
	// IncludeNonGit also reports folders that merely look like a project
	// (package.json, go.mod, ...) but are not repositories.
	IncludeNonGit bool
	// Known marks rows that are already tracked, keyed by lowercased path.
	Known map[string]bool
	// Limit caps the number of results so a scan of C:\ cannot melt the UI.
	Limit int
}

// skipDirs are never descended into. They are large, uninteresting, and
// scanning them is the difference between a scan that takes a second and one
// that takes a minute.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, "out": true, "bin": true, "obj": true,
	".venv": true, "venv": true, "env": true, "__pycache__": true,
	".next": true, ".nuxt": true, ".svelte-kit": true, ".cache": true,
	".gradle": true, ".idea": true, ".vs": true, ".tox": true,
	"Library": true, "AppData": true, "$RECYCLE.BIN": true,
	"System Volume Information": true, "Windows": true,
	"packages": true, "Pods": true, "DerivedData": true,
}

// markers maps a file name to the project kind it implies.
var markers = map[string]string{
	"package.json":     "Node",
	"go.mod":           "Go",
	"Cargo.toml":       "Rust",
	"pyproject.toml":   "Python",
	"requirements.txt": "Python",
	"setup.py":         "Python",
	"pom.xml":          "Java",
	"build.gradle":     "Gradle",
	"build.gradle.kts": "Gradle",
	"composer.json":    "PHP",
	"Gemfile":          "Ruby",
	"pubspec.yaml":     "Flutter",
	"CMakeLists.txt":   "C/C++",
	"Makefile":         "Make",
	"Dockerfile":       "Docker",
	"deno.json":        "Deno",
	"mix.exs":          "Elixir",
}

var markerSuffixes = map[string]string{
	".sln":       ".NET",
	".csproj":    ".NET",
	".xcodeproj": "Xcode",
}

// Run performs the scan. It respects ctx so a slow network drive can be
// abandoned when the client goes away.
func Run(ctx context.Context, opts Options) ([]Result, error) {
	root, err := filepath.Abs(strings.TrimSpace(opts.Root))
	if err != nil {
		return nil, err
	}
	if opts.Depth <= 0 {
		opts.Depth = 3
	}
	if opts.Depth > 8 {
		opts.Depth = 8
	}
	if opts.Limit <= 0 {
		opts.Limit = 500
	}
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}

	results := []Result{}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if len(results) >= opts.Limit || ctx.Err() != nil {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return // unreadable folder (permissions, junction loop) — just skip it
		}

		res, isProject := inspect(dir, entries)
		if isProject && (res.IsGit || opts.IncludeNonGit) {
			res.Known = opts.Known[strings.ToLower(res.Path)]
			results = append(results, res)
			// A repository's subfolders belong to it, so stop descending here.
			if res.IsGit {
				return
			}
		}
		if depth <= 0 {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if skipDirs[name] {
				continue
			}
			// Hidden folders rarely hold projects and often hold thousands of files.
			if strings.HasPrefix(name, ".") && name != ".config" {
				continue
			}
			walk(filepath.Join(dir, name), depth-1)
		}
	}
	walk(root, opts.Depth)

	sort.Slice(results, func(i, j int) bool { return results[i].Modified.After(results[j].Modified) })
	return results, ctx.Err()
}

func inspect(dir string, entries []os.DirEntry) (Result, bool) {
	res := Result{Path: dir, Name: filepath.Base(dir)}
	kinds := map[string]bool{}
	hasMarker := false

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == ".git" {
				res.IsGit = true
			}
			continue
		}
		// A worktree or submodule stores .git as a file pointing elsewhere.
		if name == ".git" {
			res.IsGit = true
			continue
		}
		if strings.EqualFold(name, "README.md") || strings.EqualFold(name, "README") {
			res.HasReadme = true
		}
		if kind, ok := markers[name]; ok {
			kinds[kind] = true
			hasMarker = true
		}
		for suffix, kind := range markerSuffixes {
			if strings.HasSuffix(strings.ToLower(name), suffix) {
				kinds[kind] = true
				hasMarker = true
			}
		}
	}

	if !res.IsGit && !hasMarker {
		return res, false
	}

	if info, err := os.Stat(dir); err == nil {
		res.Modified = info.ModTime()
	}
	// The HEAD file changes on every commit and checkout, which tracks
	// "when did I last actually work on this" far better than the folder mtime.
	if res.IsGit {
		if info, err := os.Stat(filepath.Join(dir, ".git", "HEAD")); err == nil && info.ModTime().After(res.Modified) {
			res.Modified = info.ModTime()
		}
		res.Remote = readOriginURL(dir)
	}

	for k := range kinds {
		res.Kinds = append(res.Kinds, k)
	}
	sort.Strings(res.Kinds)
	if res.Kinds == nil {
		res.Kinds = []string{}
	}
	return res, true
}

// readOriginURL parses .git/config directly instead of spawning a git process:
// a scan can turn up hundreds of repositories and a process each would be slow.
func readOriginURL(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		return ""
	}
	inOrigin := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inOrigin = strings.HasPrefix(trimmed, `[remote "origin"]`)
			continue
		}
		if inOrigin && strings.HasPrefix(trimmed, "url") {
			if _, value, ok := strings.Cut(trimmed, "="); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}
