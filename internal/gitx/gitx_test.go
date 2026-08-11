package gitx

import "testing"

func TestParseBranchLine(t *testing.T) {
	cases := []struct {
		line     string
		branch   string
		upstream string
		ahead    int
		behind   int
	}{
		{"main", "main", "", 0, 0},
		{"main...origin/main", "main", "origin/main", 0, 0},
		{"main...origin/main [ahead 3]", "main", "origin/main", 3, 0},
		{"main...origin/main [behind 2]", "main", "origin/main", 0, 2},
		{"feature/x...origin/feature/x [ahead 1, behind 4]", "feature/x", "origin/feature/x", 1, 4},
		{"No commits yet on main", "No commits yet on main", "", 0, 0},
	}

	for _, c := range cases {
		var info Info
		parseBranchLine(&info, c.line)
		if info.Branch != c.branch {
			t.Errorf("%q: branch = %q, wanted %q", c.line, info.Branch, c.branch)
		}
		if info.Upstream != c.upstream {
			t.Errorf("%q: upstream = %q, wanted %q", c.line, info.Upstream, c.upstream)
		}
		if info.Ahead != c.ahead || info.Behind != c.behind {
			t.Errorf("%q: ahead/behind = %d/%d, wanted %d/%d",
				c.line, info.Ahead, info.Behind, c.ahead, c.behind)
		}
	}
}

func TestParseStatusCountsByKind(t *testing.T) {
	out := "## main...origin/main [ahead 1]\n" +
		"M  staged.go\n" + // staged
		" M edited.go\n" + // unstaged
		"MM both.go\n" + // both
		"?? new.txt\n" + // untracked
		"UU conflicted.go\n" + // conflict
		`R  "old name.go" -> "new name.go"` + "\n"

	var info Info
	parseStatus(&info, out)

	if info.Staged != 3 { // staged + both + rename
		t.Errorf("staged = %d, wanted 3", info.Staged)
	}
	if info.Unstaged != 2 { // edited + both
		t.Errorf("unstaged = %d, wanted 2", info.Unstaged)
	}
	if info.Untracked != 1 {
		t.Errorf("untracked = %d, wanted 1", info.Untracked)
	}
	if info.Conflicted != 1 {
		t.Errorf("conflicted = %d, wanted 1", info.Conflicted)
	}
	if !info.Dirty {
		t.Error("dirty was not set")
	}
	if info.Ahead != 1 {
		t.Errorf("ahead = %d", info.Ahead)
	}

	// A rename must report the destination name, not the source.
	last := info.Changes[len(info.Changes)-1]
	if last.Path != "new name.go" {
		t.Errorf("rename path = %q", last.Path)
	}
}

func TestParseStatusCleanRepo(t *testing.T) {
	var info Info
	parseStatus(&info, "## main...origin/main\n")
	if info.Dirty {
		t.Error("a clean repository was reported as dirty")
	}
}

func TestParseLog(t *testing.T) {
	record := func(fields ...string) string {
		out := ""
		for i, f := range fields {
			if i > 0 {
				out += fieldSep
			}
			out += f
		}
		return out + recordSep
	}
	// The author name and subject are deliberately non-ASCII: git hands us raw
	// UTF-8, and a parser that slices on bytes would corrupt them.
	out := record("abc123def", "abc123d", "İlk commit", "Ayşe", "a@b.c", "2026-08-10T14:49:03+03:00", "HEAD -> main", "Body\nsecond line") +
		"\n" + record("999", "999", "Next", "Mehmet", "m@b.c", "2026-08-09T10:00:00+03:00", "", "")

	commits := parseLog(out)
	if len(commits) != 2 {
		t.Fatalf("parsed %d commits, wanted 2", len(commits))
	}
	if commits[0].Subject != "İlk commit" {
		t.Errorf("subject = %q", commits[0].Subject)
	}
	if commits[0].Author != "Ayşe" {
		t.Errorf("author = %q", commits[0].Author)
	}
	if commits[0].Body != "Body\nsecond line" {
		t.Errorf("body = %q", commits[0].Body)
	}
	if commits[0].Date.Year() != 2026 || commits[0].Date.Day() != 10 {
		t.Errorf("date did not parse: %v", commits[0].Date)
	}
	if commits[1].Refs != "" {
		t.Errorf("empty refs field = %q", commits[1].Refs)
	}
}

func TestParseLogIgnoresEmptyOutput(t *testing.T) {
	if got := parseLog(""); len(got) != 0 {
		t.Errorf("empty output produced %d commits", len(got))
	}
}
