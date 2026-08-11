package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAddUsesFolderNameByDefault(t *testing.T) {
	s := newStore(t)
	dir := filepath.Join(t.TempDir(), "portfolio-site")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	p, err := s.Add(Project{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "portfolio-site" {
		t.Errorf("name = %q, wanted the folder name", p.Name)
	}
	if p.ID == "" {
		t.Error("no id was assigned")
	}
}

// TestAddRejectsDuplicates protects the invariant the whole UI leans on: one
// row per folder on disk, however the path was spelled.
func TestAddRejectsDuplicates(t *testing.T) {
	s := newStore(t)
	dir := t.TempDir()

	if _, err := s.Add(Project{Path: dir}); err != nil {
		t.Fatal(err)
	}
	// The same folder spelled differently: a trailing separator and a "." part.
	if _, err := s.Add(Project{Path: dir + string(filepath.Separator) + "."}); err == nil {
		t.Error("the same folder was added twice")
	}
}

func TestAddRejectsMissingAndFiles(t *testing.T) {
	s := newStore(t)

	if _, err := s.Add(Project{Path: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("a folder that does not exist was accepted")
	}

	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Project{Path: file}); err == nil {
		t.Error("a file was accepted as a project")
	}
}

func TestUpdateAppliesOnlyProvidedFields(t *testing.T) {
	s := newStore(t)
	p, err := s.Add(Project{Path: t.TempDir(), Name: "old", Notes: "note"})
	if err != nil {
		t.Fatal(err)
	}

	name := "new"
	updated, err := s.Update(p.ID, Patch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "new" {
		t.Errorf("the name was not updated: %q", updated.Name)
	}
	if updated.Notes != "note" {
		t.Errorf("a field that was not touched got cleared: %q", updated.Notes)
	}
}

func TestTagsAreCleanedAndCounted(t *testing.T) {
	s := newStore(t)
	if _, err := s.Add(Project{Path: t.TempDir(), Tags: []string{" work ", "web", "work", ""}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Project{Path: t.TempDir(), Tags: []string{"web"}}); err != nil {
		t.Fatal(err)
	}

	counts := map[string]int{}
	for _, tc := range s.Tags() {
		counts[tc.Name] = tc.Count
	}
	if counts["web"] != 2 {
		t.Errorf("web = %d, wanted 2", counts["web"])
	}
	if counts["work"] != 1 {
		t.Errorf("work = %d (trimming or deduplication did not run)", counts["work"])
	}
}

func TestFavoritesSortFirst(t *testing.T) {
	s := newStore(t)
	a, _ := s.Add(Project{Path: t.TempDir(), Name: "a"})
	b, _ := s.Add(Project{Path: t.TempDir(), Name: "b"})

	yes := true
	if _, err := s.Update(b.ID, Patch{Favorite: &yes}); err != nil {
		t.Fatal(err)
	}
	list := s.Projects()
	if list[0].ID != b.ID {
		t.Errorf("the favourite is not at the top (first: %s, wanted: %s)", list[0].Name, "b")
	}
	_ = a
}

func TestDataSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Project{Path: t.TempDir(), Name: "persistent"}); err != nil {
		t.Fatal(err)
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	list := again.Projects()
	if len(list) != 1 || list[0].Name != "persistent" {
		t.Errorf("data was lost on reopen: %+v", list)
	}
}

// TestCorruptFileIsBackedUp: a broken data.json must never be silently thrown
// away — the user's whole project list lives in it.
func TestCorruptFileIsBackedUp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err == nil {
		t.Error("no warning was returned for a corrupt file")
	}
	if s == nil {
		t.Fatal("the store would not open on a corrupt file; the app could not start")
	}

	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".corrupt-") {
			found = true
		}
	}
	if !found {
		t.Error("the corrupt file was not backed up")
	}
}

func TestNormalizePath(t *testing.T) {
	if _, err := NormalizePath("   "); err == nil {
		t.Error("an empty path was accepted")
	}
	// Paths pasted with quotes ("Copy as path" on Windows) have to work.
	got, err := NormalizePath(`"` + t.TempDir() + `"`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(got, `"`) {
		t.Errorf("the quotes were not stripped: %q", got)
	}
}
