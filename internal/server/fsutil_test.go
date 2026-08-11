package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSafeJoinRejectsEscapes is the important one: every one of these inputs
// would hand out files outside the project folder if the check regressed.
func TestSafeJoinRejectsEscapes(t *testing.T) {
	base := t.TempDir()

	escapes := []string{
		"..",
		"../",
		"../etc/passwd",
		"sub/../../outside",
		`..\windows\system32`,
		"foo/../../..",
	}
	for _, rel := range escapes {
		if got, err := safeJoin(base, rel); err == nil {
			t.Errorf("safeJoin(%q) allowed %q — it should have been rejected", rel, got)
		}
	}
}

func TestSafeJoinAllowsInside(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "src", "app"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A leading slash means "relative to the project root", not an absolute
	// path: even "/etc/passwd" resolves to <project>/etc/passwd and stays in.
	cases := map[string]string{
		"":                 base,
		".":                base,
		"src":              filepath.Join(base, "src"),
		"src/app":          filepath.Join(base, "src", "app"),
		"/src/app":         filepath.Join(base, "src", "app"),
		`src\app`:          filepath.Join(base, "src", "app"), // Windows separator is accepted too
		"/etc/passwd":      filepath.Join(base, "etc", "passwd"),
		"missing/file.txt": filepath.Join(base, "missing", "file.txt"),
	}
	for rel, want := range cases {
		got, err := safeJoin(base, rel)
		if err != nil {
			t.Errorf("safeJoin(%q) unexpected error: %v", rel, err)
			continue
		}
		if got != want {
			t.Errorf("safeJoin(%q) = %q, wanted %q", rel, got, want)
		}
	}
}

// TestSafeJoinFollowsSymlinks covers the case a lexical prefix check misses:
// a link inside the project that points somewhere else entirely.
func TestSafeJoinFollowsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink on Windows requires administrator rights")
	}
	base := t.TempDir()
	outside := t.TempDir()

	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Fatal(err)
	}
	if got, err := safeJoin(base, "escape"); err == nil {
		t.Errorf("a symlink led outside the project but was allowed: %q", got)
	}
}

func TestReadTextFileRejectsBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.bin")
	if err := os.WriteFile(path, []byte{0x89, 'P', 'N', 'G', 0x00, 0x1a}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readTextFile(path, 1024); err == nil {
		t.Error("a binary file was read as text")
	}
}

func TestReadTextFileTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 5000)), 0o644); err != nil {
		t.Fatal(err)
	}

	content, truncated, err := readTextFile(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Error("the truncated flag was not set")
	}
	if len(content) != 100 {
		t.Errorf("content is %d bytes, wanted 100", len(content))
	}
}

// TestReadTextFileKeepsUTF8Valid guards the boundary case where the byte limit
// lands in the middle of a multi-byte character — common in any non-English text.
func TestReadTextFileKeepsUTF8Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utf8.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("ğ", 50)), 0o644); err != nil {
		t.Fatal(err)
	}

	// 'ğ' is two bytes, so a 25 byte limit cuts one character in half.
	content, _, err := readTextFile(path, 25)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range content {
		if r == '�' {
			t.Fatal("the output contains a replacement character")
		}
	}
}
