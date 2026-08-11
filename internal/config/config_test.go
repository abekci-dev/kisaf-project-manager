package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateLegacyDataCarriesProjectsOver is the guard against the rename
// silently losing someone's project list: the folder moved with the program
// name, so an existing install must be picked up from the old location.
func TestMigrateLegacyDataCarriesProjectsOver(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	old := filepath.Join(home, ".config", "bekci")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "data.json"),
		[]byte(`{"version":1,"projects":[{"id":"a","name":"my-app"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "config.json"),
		[]byte(`{"host":"bekci","port":80,"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	newDir := filepath.Join(home, ".config", "kisaf")
	from, ok := migrateLegacyData(newDir)
	if !ok {
		t.Fatal("migration did not run")
	}
	if from != old {
		t.Errorf("migrated from %q, wanted %q", from, old)
	}

	moved, err := os.ReadFile(filepath.Join(newDir, "data.json"))
	if err != nil {
		t.Fatalf("project list did not arrive: %v", err)
	}
	if want := "my-app"; !contains(string(moved), want) {
		t.Errorf("project list lost its contents: %s", moved)
	}

	// The old folder is a backup, not a casualty.
	if _, err := os.Stat(filepath.Join(old, "data.json")); err != nil {
		t.Error("the old folder was destroyed; it should be left as a backup")
	}
}

// TestMigrationRenamesHost: a carried-over config would keep announcing the
// previous mDNS name, so http://kisaf.local would not resolve after upgrading.
func TestMigrationRenamesHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	old := filepath.Join(home, ".config", "bekci")
	_ = os.MkdirAll(old, 0o755)
	_ = os.WriteFile(filepath.Join(old, "data.json"), []byte(`{"version":1}`), 0o600)
	_ = os.WriteFile(filepath.Join(old, "config.json"),
		[]byte(`{"host":"bekci","port":7777,"token":"keep-me"}`), 0o600)

	newDir := filepath.Join(home, ".config", "kisaf")
	if _, ok := migrateLegacyData(newDir); !ok {
		t.Fatal("migration did not run")
	}

	raw, err := os.ReadFile(filepath.Join(newDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["host"] != "kisaf" {
		t.Errorf("host = %v, wanted kisaf", doc["host"])
	}
	// Everything the user actually chose must survive.
	if doc["token"] != "keep-me" {
		t.Errorf("token was lost: %v", doc["token"])
	}
	if doc["port"] != float64(7777) {
		t.Errorf("port was lost: %v", doc["port"])
	}
}

// TestMigrationKeepsCustomHost: someone who deliberately renamed their instance
// should not have that undone by the upgrade.
func TestMigrationKeepsCustomHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	old := filepath.Join(home, ".config", "bekci")
	_ = os.MkdirAll(old, 0o755)
	_ = os.WriteFile(filepath.Join(old, "data.json"), []byte(`{"version":1}`), 0o600)
	_ = os.WriteFile(filepath.Join(old, "config.json"), []byte(`{"host":"projects"}`), 0o600)

	newDir := filepath.Join(home, ".config", "kisaf")
	migrateLegacyData(newDir)

	raw, _ := os.ReadFile(filepath.Join(newDir, "config.json"))
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	if doc["host"] != "projects" {
		t.Errorf("a deliberately chosen host was overwritten: %v", doc["host"])
	}
}

// TestMigrationSkipsWhenAlreadyPresent: running twice must not overwrite the
// current data with a stale copy of the old one.
func TestMigrationSkipsWhenAlreadyPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	old := filepath.Join(home, ".config", "bekci")
	_ = os.MkdirAll(old, 0o755)
	_ = os.WriteFile(filepath.Join(old, "data.json"), []byte(`{"marker":"old"}`), 0o600)

	newDir := filepath.Join(home, ".config", "kisaf")
	_ = os.MkdirAll(newDir, 0o755)
	_ = os.WriteFile(filepath.Join(newDir, "data.json"), []byte(`{"marker":"current"}`), 0o600)

	if _, ok := migrateLegacyData(newDir); ok {
		t.Error("migration ran over existing data")
	}
	raw, _ := os.ReadFile(filepath.Join(newDir, "data.json"))
	if !contains(string(raw), "current") {
		t.Errorf("current data was overwritten: %s", raw)
	}
}

func TestMigrationNoOpOnFreshInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	if _, ok := migrateLegacyData(filepath.Join(home, ".config", "kisaf")); ok {
		t.Error("migration reported success with nothing to migrate")
	}
}

func TestDefaultsUseTheNewName(t *testing.T) {
	if got := Default().Host; got != "kisaf" {
		t.Errorf("default host = %q", got)
	}
	if got := (Config{Host: "kisaf"}).MDNSName(); got != "kisaf.local" {
		t.Errorf("mDNS name = %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
