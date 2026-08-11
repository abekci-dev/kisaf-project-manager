package server

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
)

// safeJoin resolves rel underneath base and refuses to leave it.
//
// Symlinks are resolved first, because "projects/link" pointing at C:\Windows
// would otherwise pass a naive prefix check and hand out files the user never
// meant to expose.
func safeJoin(base, rel string) (string, error) {
	base = filepath.Clean(base)
	rel = strings.TrimSpace(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return base, nil
	}
	if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", apperr.New(apperr.CodePathInvalid, "invalid path")
	}

	joined := filepath.Join(base, filepath.FromSlash(rel))

	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		realBase = base
	}
	realJoined, err := filepath.EvalSymlinks(joined)
	if err != nil {
		// The target may legitimately not exist yet; fall back to the lexical
		// path, which the checks below still cover.
		realJoined = joined
	}

	if !withinDir(realBase, realJoined) {
		return "", apperr.New(apperr.CodePathOutside, "cannot leave the project folder")
	}
	return joined, nil
}

func withinDir(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// readTextFile reads at most limit bytes and refuses anything that is not text,
// so clicking a 300 MB binary in the file tree cannot wedge the browser.
func readTextFile(path string, limit int64) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()

	buf := make([]byte, limit+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", false, err
	}
	data := buf[:n]

	truncated := int64(n) > limit
	if truncated {
		data = data[:limit]
	}
	if isBinary(data) {
		return "", false, apperr.New(apperr.CodeFileBinary, "this file is not text (binary content)")
	}
	// A cut at exactly the limit can slice a multi-byte rune in half.
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data), truncated, nil
}

// isBinary uses the same heuristic as git: a NUL byte in the first 8 KB.
func isBinary(data []byte) bool {
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	for _, b := range head {
		if b == 0 {
			return true
		}
	}
	return false
}

// rootFolders returns the starting points offered by the folder picker:
// drive letters on Windows, a few well-known folders everywhere.
func rootFolders() []browseEntry {
	out := []browseEntry{}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, browseEntry{Name: "Home", Path: home})
		for _, sub := range []string{"Desktop", "Documents", "Downloads", "Projects", "Projeler", "source", "repos", "dev", "git"} {
			p := filepath.Join(home, sub)
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				out = append(out, browseEntry{Name: sub, Path: p})
			}
		}
	}
	if runtime.GOOS == "windows" {
		for letter := 'A'; letter <= 'Z'; letter++ {
			drive := string(letter) + `:\`
			if _, err := os.Stat(drive); err == nil {
				out = append(out, browseEntry{Name: string(letter) + ":", Path: drive})
			}
		}
	} else {
		out = append(out, browseEntry{Name: "/", Path: "/"})
	}
	return out
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return strings.ToUpper(runtime.GOOS[:1]) + runtime.GOOS[1:]
	}
}
