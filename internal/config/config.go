// Package config resolves runtime settings from a JSON file, environment
// variables and command line flags. Everything has a sane default so the
// binary can be double-clicked with no setup at all.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Config holds process level settings. These are deliberately kept out of the
// web UI: they decide who may talk to the server, so they live in a file only
// someone with disk access can edit.
type Config struct {
	// Host is the name announced over mDNS/LLMNR. "kisaf" makes both
	// http://kisaf.local and (on Windows) http://kisaf resolve.
	Host string `json:"host"`
	// Port is tried first; the first free FallbackPorts entry is used if it
	// is taken. Port 80 is what makes the URL work without a ":1234" suffix.
	Port          int   `json:"port"`
	FallbackPorts []int `json:"fallbackPorts"`
	// Bind is the listen address. 0.0.0.0 is required for the .local name to
	// work from phones/other machines; remote requests still need Token.
	Bind string `json:"bind"`
	// Token gates every request that does not come from this machine. Empty
	// means "loopback only" — remote callers get a 403 explaining the fix.
	Token       string `json:"token"`
	EnableMDNS  bool   `json:"enableMDNS"`
	EnableTray  bool   `json:"enableTray"`
	OpenBrowser bool   `json:"openBrowser"`
	// AllowedHosts are extra Host header values accepted on top of localhost,
	// the mDNS name and bare IPs. Needed when a homelab reverse proxy fronts
	// this with a real domain name.
	AllowedHosts []string `json:"allowedHosts"`

	// DataDir is where data.json, the log and the config itself live.
	DataDir string `json:"-"`
	// Path of the config file that produced this value.
	Path string `json:"-"`
	// MigratedFrom is the pre-rename data folder this install was carried over
	// from, if any. Empty on a normal start.
	MigratedFrom string `json:"-"`
}

// Default returns the configuration used on a fresh install.
func Default() Config {
	return Config{
		Host:          "kisaf",
		Port:          80,
		FallbackPorts: []int{7777, 8777, 8080, 0},
		Bind:          "0.0.0.0",
		Token:         "",
		EnableMDNS:    true,
		EnableTray:    true,
		OpenBrowser:   true,
		AllowedHosts:  []string{},
	}
}

// DataDir returns the per-user directory holding all state. It honours
// KISAF_DATA_DIR so a portable/USB install can keep everything together.
func DataDir() string {
	if v := strings.TrimSpace(os.Getenv("KISAF_DATA_DIR")); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "kisaf")
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "kisaf")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".kisaf"
	}
	return filepath.Join(home, ".kisaf")
}

// legacyDataDirs are the folders used before the program was renamed to kisaf.
// They are only ever read, never written.
func legacyDataDirs() []string {
	var dirs []string
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			dirs = append(dirs, filepath.Join(appData, "Bekci"))
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "bekci"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".bekci"))
	}
	return dirs
}

// migrateLegacyData copies a pre-rename project list into the new data folder.
//
// Renaming the program moved the data folder with it, which would otherwise
// greet an existing user with an empty list and no hint that their projects are
// still on disk. The old folder is copied rather than moved, so it stays intact
// as a backup if anything about the new layout goes wrong.
func migrateLegacyData(newDir string) (string, bool) {
	if _, err := os.Stat(filepath.Join(newDir, "data.json")); err == nil {
		return "", false // already migrated, or a fresh install that has run once
	}

	for _, old := range legacyDataDirs() {
		if old == newDir {
			continue
		}
		if _, err := os.Stat(filepath.Join(old, "data.json")); err != nil {
			continue
		}
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			return "", false
		}
		// Only the two files that hold user intent. The log and the extracted
		// icon are regenerated, and copying them would just carry stale state.
		copied := false
		for _, name := range []string{"data.json", "config.json"} {
			raw, err := os.ReadFile(filepath.Join(old, name))
			if err != nil {
				continue
			}
			if name == "config.json" {
				raw = renameHostInConfig(raw)
			}
			if err := os.WriteFile(filepath.Join(newDir, name), raw, 0o600); err == nil {
				copied = true
			}
		}
		if copied {
			return old, true
		}
	}
	return "", false
}

// renameHostInConfig updates the mDNS name carried over from the old install.
// A migrated config would otherwise keep announcing the previous name, and
// http://kisaf.local would not resolve on a machine that had upgraded.
func renameHostInConfig(raw []byte) []byte {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	if host, ok := doc["host"].(string); ok && strings.EqualFold(host, "bekci") {
		doc["host"] = "kisaf"
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return raw
	}
	return append(out, '\n')
}

// Load reads config.json from the data directory, creating it with defaults on
// first run. Environment variables (KISAF_PORT, KISAF_HOST, KISAF_TOKEN,
// KISAF_BIND) win over the file so container/homelab deploys stay declarative.
//
// MigratedFrom reports the old folder when a pre-rename install was carried
// over, so the caller can say so in the log.
func Load() (Config, error) {
	cfg := Default()
	cfg.DataDir = DataDir()
	cfg.Path = filepath.Join(cfg.DataDir, "config.json")

	if from, ok := migrateLegacyData(cfg.DataDir); ok {
		cfg.MigratedFrom = from
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return cfg, err
	}

	raw, err := os.ReadFile(cfg.Path)
	switch {
	case err == nil:
		// Unmarshal on top of the defaults so new fields added by a future
		// version keep working with an old config file.
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, err
		}
	case os.IsNotExist(err):
		if err := cfg.Save(); err != nil {
			return cfg, err
		}
	default:
		return cfg, err
	}

	applyEnv(&cfg)
	normalize(&cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := strings.TrimSpace(os.Getenv("KISAF_HOST")); v != "" {
		cfg.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_BIND")); v != "" {
		cfg.Bind = v
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_TOKEN")); v != "" {
		cfg.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 65535 {
			cfg.Port = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_NO_MDNS")); v == "1" {
		cfg.EnableMDNS = false
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_NO_TRAY")); v == "1" {
		cfg.EnableTray = false
	}
	if v := strings.TrimSpace(os.Getenv("KISAF_NO_BROWSER")); v == "1" {
		cfg.OpenBrowser = false
	}
}

func normalize(cfg *Config) {
	cfg.Host = strings.ToLower(strings.TrimSpace(cfg.Host))
	cfg.Host = strings.TrimSuffix(cfg.Host, ".local")
	if cfg.Host == "" {
		cfg.Host = "kisaf"
	}
	if cfg.Bind == "" {
		cfg.Bind = "0.0.0.0"
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		cfg.Port = 80
	}
	if len(cfg.FallbackPorts) == 0 {
		cfg.FallbackPorts = Default().FallbackPorts
	}
}

// Save writes the config back to disk, pretty printed so it stays hand-editable.
func (c Config) Save() error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.Path)
}

// MDNSName is the fully qualified name announced on the local network.
func (c Config) MDNSName() string { return c.Host + ".local" }
