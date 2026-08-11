//go:build windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
	"github.com/abekci-dev/kisaf-project-manager/internal/proc"
)

// knownEditor describes where to look for one program. Paths may contain
// globs and use {LOCALAPPDATA} style placeholders for the usual Windows roots.
type knownEditor struct {
	id    string
	name  string
	paths []string
	// cliNames are looked up on PATH when none of the paths match; this is how
	// a "code" shim installed by the VS Code setup gets picked up.
	cliNames []string
	args     []string
}

var windowsEditors = []knownEditor{
	{
		id:   "vscode",
		name: "Visual Studio Code",
		paths: []string{
			`{LOCALAPPDATA}\Programs\Microsoft VS Code\Code.exe`,
			`{PROGRAMFILES}\Microsoft VS Code\Code.exe`,
			`{PROGRAMFILES86}\Microsoft VS Code\Code.exe`,
		},
		cliNames: []string{"code"},
	},
	{
		id:   "vscode-insiders",
		name: "VS Code Insiders",
		paths: []string{
			`{LOCALAPPDATA}\Programs\Microsoft VS Code Insiders\Code - Insiders.exe`,
			`{PROGRAMFILES}\Microsoft VS Code Insiders\Code - Insiders.exe`,
		},
		cliNames: []string{"code-insiders"},
	},
	{
		id:   "cursor",
		name: "Cursor",
		paths: []string{
			`{LOCALAPPDATA}\Programs\cursor\Cursor.exe`,
			`{PROGRAMFILES}\Cursor\Cursor.exe`,
		},
		cliNames: []string{"cursor"},
	},
	{
		id:   "windsurf",
		name: "Windsurf",
		paths: []string{
			`{LOCALAPPDATA}\Programs\Windsurf\Windsurf.exe`,
			`{PROGRAMFILES}\Windsurf\Windsurf.exe`,
		},
		cliNames: []string{"windsurf"},
	},
	{
		id:       "vscodium",
		name:     "VSCodium",
		paths:    []string{`{LOCALAPPDATA}\Programs\VSCodium\VSCodium.exe`, `{PROGRAMFILES}\VSCodium\VSCodium.exe`},
		cliNames: []string{"codium"},
	},
	{
		id:       "zed",
		name:     "Zed",
		paths:    []string{`{LOCALAPPDATA}\Zed\Zed.exe`, `{LOCALAPPDATA}\Programs\Zed\Zed.exe`},
		cliNames: []string{"zed"},
	},
	{
		id:       "sublime",
		name:     "Sublime Text",
		paths:    []string{`{PROGRAMFILES}\Sublime Text\sublime_text.exe`, `{PROGRAMFILES}\Sublime Text 3\sublime_text.exe`},
		cliNames: []string{"subl"},
	},
	{
		id:    "notepadpp",
		name:  "Notepad++",
		paths: []string{`{PROGRAMFILES}\Notepad++\notepad++.exe`, `{PROGRAMFILES86}\Notepad++\notepad++.exe`},
	},
	{
		id:    "visualstudio",
		name:  "Visual Studio",
		paths: []string{`{PROGRAMFILES}\Microsoft Visual Studio\*\*\Common7\IDE\devenv.exe`, `{PROGRAMFILES86}\Microsoft Visual Studio\*\*\Common7\IDE\devenv.exe`},
	},
	{
		id:    "fleet",
		name:  "JetBrains Fleet",
		paths: []string{`{LOCALAPPDATA}\Programs\Fleet\Fleet.exe`},
	},
}

// jetBrainsProducts maps the launcher file name JetBrains uses to a display
// name. Toolbox writes shims into a scripts folder; standalone installs put a
// matching exe under bin.
var jetBrainsProducts = map[string]string{
	"idea":      "IntelliJ IDEA",
	"idea64":    "IntelliJ IDEA",
	"pycharm":   "PyCharm",
	"pycharm64": "PyCharm",
	"webstorm":  "WebStorm",
	"goland":    "GoLand",
	"rider":     "Rider",
	"rider64":   "Rider",
	"clion":     "CLion",
	"clion64":   "CLion",
	"phpstorm":  "PhpStorm",
	"rubymine":  "RubyMine",
	"datagrip":  "DataGrip",
	"rustrover": "RustRover",
	"studio":    "Android Studio",
	"studio64":  "Android Studio",
	"dataspell": "DataSpell",
	"aqua":      "Aqua",
}

func expandRoots(p string) string {
	repl := strings.NewReplacer(
		"{LOCALAPPDATA}", os.Getenv("LOCALAPPDATA"),
		"{APPDATA}", os.Getenv("APPDATA"),
		"{PROGRAMFILES}", os.Getenv("ProgramFiles"),
		"{PROGRAMFILES86}", os.Getenv("ProgramFiles(x86)"),
		"{USERPROFILE}", os.Getenv("USERPROFILE"),
	)
	return repl.Replace(p)
}

func detectEditors() []Editor {
	out := []Editor{}
	seen := map[string]bool{}

	add := func(e Editor) {
		key := strings.ToLower(e.Exec)
		if e.Exec == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, e)
	}

	for _, ke := range windowsEditors {
		paths := make([]string, 0, len(ke.paths))
		for _, p := range ke.paths {
			paths = append(paths, expandRoots(p))
		}
		exe := firstExisting(paths...)
		if exe == "" {
			for _, cli := range ke.cliNames {
				if exe = lookPath(cli); exe != "" {
					break
				}
			}
		}
		if exe == "" {
			continue
		}
		add(Editor{ID: ke.id, Name: ke.name, Exec: exe, Args: ke.args, Kind: "editor", Detected: true})
	}

	out = append(out, detectJetBrains(seen)...)

	// Anything the user installed somewhere unusual can still be reached via a
	// PATH shim; this catches Toolbox's "shell scripts" directory too.
	for cli, name := range map[string]string{
		"idea": "IntelliJ IDEA", "pycharm": "PyCharm", "webstorm": "WebStorm",
		"goland": "GoLand", "rider": "Rider", "clion": "CLion",
		"phpstorm": "PhpStorm", "rubymine": "RubyMine", "datagrip": "DataGrip",
		"rustrover": "RustRover",
	} {
		if exe := lookPath(cli); exe != "" {
			add(Editor{ID: "jb-" + cli, Name: name, Exec: exe, Kind: "editor", Detected: true})
		}
	}

	return out
}

func detectJetBrains(seen map[string]bool) []Editor {
	out := []Editor{}
	patterns := []string{
		expandRoots(`{LOCALAPPDATA}\JetBrains\Toolbox\scripts\*.cmd`),
		expandRoots(`{LOCALAPPDATA}\JetBrains\Toolbox\scripts\*.bat`),
		expandRoots(`{PROGRAMFILES}\JetBrains\*\bin\*64.exe`),
		expandRoots(`{PROGRAMFILES86}\JetBrains\*\bin\*64.exe`),
		expandRoots(`{PROGRAMFILES}\Android\Android Studio\bin\studio64.exe`),
		expandRoots(`{LOCALAPPDATA}\Programs\*\bin\*64.exe`),
	}

	// One entry per product: Toolbox happily keeps three IntelliJ builds around
	// and listing all of them would only make the dropdown useless.
	byProduct := map[string]string{}
	for _, pattern := range patterns {
		matches, _ := globSorted(pattern)
		for _, m := range matches {
			base := strings.ToLower(strings.TrimSuffix(filepath.Base(m), filepath.Ext(m)))
			name, ok := jetBrainsProducts[base]
			if !ok {
				continue
			}
			if _, exists := byProduct[name]; exists {
				continue
			}
			byProduct[name] = m
		}
	}

	for name, exe := range byProduct {
		key := strings.ToLower(exe)
		if seen[key] {
			continue
		}
		seen[key] = true
		id := "jb-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
		out = append(out, Editor{ID: id, Name: name, Exec: exe, Kind: "editor", Detected: true})
	}
	return out
}

// buildDetached prepares a command that outlives us. Batch files cannot be
// started directly by CreateProcess, so they go through cmd.exe — which is
// exactly what JetBrains Toolbox shims need.
func buildDetached(exe string, args []string) *exec.Cmd {
	ext := strings.ToLower(filepath.Ext(exe))
	if ext == ".cmd" || ext == ".bat" {
		full := append([]string{"/c", exe}, args...)
		cmd := exec.Command("cmd.exe", full...)
		proc.Hide(cmd)
		return cmd
	}
	cmd := exec.Command(exe, args...)
	proc.Detach(cmd)
	return cmd
}

// reveal opens Explorer with the folder selected inside its parent — the exact
// behaviour of "Show in Explorer" in an IDE.
//
// explorer.exe is famously picky about how /select is quoted, so the command
// line is written by hand instead of letting Go quote the arguments.
func reveal(path string) error {
	path = filepath.Clean(path)
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	if err := cmd.Start(); err != nil {
		return err
	}
	// explorer.exe returns exit status 1 even on success, so the result of
	// Wait is deliberately ignored.
	go func() { _ = cmd.Wait() }()
	return nil
}

// openFolder opens the folder itself in Explorer.
func openFolder(path string) error {
	path = filepath.Clean(path)
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe "` + path + `"`}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openTerminal prefers Windows Terminal, then PowerShell, then cmd.
func openTerminal(path, prefer string) error {
	type candidate struct {
		id   string
		exe  string
		args []string
	}
	candidates := []candidate{
		{"wt", lookPath("wt.exe"), []string{"-d", path}},
		{"pwsh", lookPath("pwsh.exe"), []string{"-NoExit", "-Command", "Set-Location -LiteralPath " + psQuote(path)}},
		{"powershell", lookPath("powershell.exe"), []string{"-NoExit", "-Command", "Set-Location -LiteralPath " + psQuote(path)}},
		{"cmd", lookPath("cmd.exe"), []string{"/K", "cd /d " + path}},
	}

	if prefer != "" {
		for i, c := range candidates {
			if c.id == prefer && c.exe != "" {
				candidates[0], candidates[i] = candidates[i], candidates[0]
				break
			}
		}
	}

	for _, c := range candidates {
		if c.exe == "" {
			continue
		}
		cmd := exec.Command(c.exe, c.args...)
		cmd.Dir = path
		proc.Console(cmd)
		if err := cmd.Start(); err == nil {
			go func() { _ = cmd.Wait() }()
			return nil
		}
	}
	return apperr.New(apperr.CodeTerminalNotFound, "no terminal found (wt / powershell / cmd)")
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func openURL(url string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	proc.Hide(cmd)
	return start(cmd)
}
