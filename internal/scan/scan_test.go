package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree builds a folder layout from a map of relative path -> file contents.
// A path ending in "/" is created as an empty directory.
func tree(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range layout {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func paths(results []Result) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = filepath.Base(r.Path)
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestFindsGitRepos(t *testing.T) {
	root := tree(t, map[string]string{
		"site/.git/HEAD":     "ref: refs/heads/main\n",
		"site/package.json":  "{}",
		"tool/.git/HEAD":     "ref: refs/heads/main\n",
		"notes/readme.txt":   "not a project",
		"deep/a/b/.git/HEAD": "ref: refs/heads/main\n",
	})

	results, err := Run(context.Background(), Options{Root: root, Depth: 4})
	if err != nil {
		t.Fatal(err)
	}
	found := paths(results)

	for _, want := range []string{"site", "tool", "b"} {
		if !contains(found, want) {
			t.Errorf("%q not found, got: %v", want, found)
		}
	}
	if contains(found, "notes") {
		t.Error("a folder that is not a git repository was reported")
	}
}

// TestDoesNotDescendIntoRepos: once a repository is found its subfolders belong
// to it, and reporting them separately would flood the import list.
func TestDoesNotDescendIntoRepos(t *testing.T) {
	root := tree(t, map[string]string{
		"outer/.git/HEAD":          "ref: refs/heads/main\n",
		"outer/inner/.git/HEAD":    "ref: refs/heads/main\n",
		"outer/inner/package.json": "{}",
	})

	results, err := Run(context.Background(), Options{Root: root, Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || filepath.Base(results[0].Path) != "outer" {
		t.Errorf("a nested repository was reported separately: %v", paths(results))
	}
}

func TestSkipsNoiseDirectories(t *testing.T) {
	root := tree(t, map[string]string{
		"app/package.json":                  "{}",
		"app/node_modules/pkg/.git/HEAD":    "ref: refs/heads/main\n",
		"app/node_modules/pkg/package.json": "{}",
	})

	results, err := Run(context.Background(), Options{Root: root, Depth: 5, IncludeNonGit: true})
	if err != nil {
		t.Fatal(err)
	}
	if contains(paths(results), "pkg") {
		t.Error("the scan descended into node_modules")
	}
}

func TestIncludeNonGitFlag(t *testing.T) {
	root := tree(t, map[string]string{"npm-only/package.json": "{}"})

	off, err := Run(context.Background(), Options{Root: root, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(off) != 0 {
		t.Errorf("a non-git project was reported with the flag off: %v", paths(off))
	}

	on, err := Run(context.Background(), Options{Root: root, Depth: 2, IncludeNonGit: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(on) != 1 {
		t.Fatalf("%d results with the flag on, wanted 1", len(on))
	}
	if !contains(on[0].Kinds, "Node") {
		t.Errorf("project kind was not recognised: %v", on[0].Kinds)
	}
}

func TestMarksKnownPaths(t *testing.T) {
	root := tree(t, map[string]string{"site/.git/HEAD": "ref: refs/heads/main\n"})
	known := map[string]bool{strings.ToLower(filepath.Join(root, "site")): true}

	results, err := Run(context.Background(), Options{Root: root, Depth: 2, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("%d results", len(results))
	}
	if !results[0].Known {
		t.Error("an already added project was not marked as known")
	}
}

func TestReadsOriginURL(t *testing.T) {
	root := tree(t, map[string]string{
		"site/.git/HEAD": "ref: refs/heads/main\n",
		"site/.git/config": `[core]
	bare = false
[remote "upstream"]
	url = https://example.com/wrong.git
[remote "origin"]
	url = https://github.com/user/site.git
`,
	})

	results, err := Run(context.Background(), Options{Root: root, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Remote != "https://github.com/user/site.git" {
		t.Errorf("origin url = %q", results[0].Remote)
	}
}

func TestDetectsGitFileWorktree(t *testing.T) {
	// In submodules and worktrees .git is a file, not a directory.
	root := tree(t, map[string]string{"wt/.git": "gitdir: /somewhere/else\n"})

	results, err := Run(context.Background(), Options{Root: root, Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].IsGit {
		t.Errorf("a worktree with a .git file was not recognised: %v", results)
	}
}

func TestRespectsCancellation(t *testing.T) {
	root := tree(t, map[string]string{"a/.git/HEAD": "x", "b/.git/HEAD": "x"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Run(ctx, Options{Root: root, Depth: 3}); err == nil {
		t.Error("a cancelled context did not produce an error")
	}
}
