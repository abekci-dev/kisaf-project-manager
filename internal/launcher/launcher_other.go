//go:build !windows

package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abekci-dev/kisaf-project-manager/internal/apperr"
	"github.com/abekci-dev/kisaf-project-manager/internal/proc"
)

type knownEditor struct {
	id       string
	name     string
	paths    []string
	cliNames []string
	args     []string
}

var unixEditors = []knownEditor{
	{id: "vscode", name: "Visual Studio Code", cliNames: []string{"code"}, paths: []string{
		"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		"/usr/share/code/bin/code",
		"/snap/bin/code",
	}},
	{id: "vscode-insiders", name: "VS Code Insiders", cliNames: []string{"code-insiders"}},
	{id: "cursor", name: "Cursor", cliNames: []string{"cursor"}, paths: []string{
		"/Applications/Cursor.app/Contents/Resources/app/bin/cursor",
	}},
	{id: "windsurf", name: "Windsurf", cliNames: []string{"windsurf"}},
	{id: "vscodium", name: "VSCodium", cliNames: []string{"codium"}},
	{id: "zed", name: "Zed", cliNames: []string{"zed"}, paths: []string{"/Applications/Zed.app/Contents/MacOS/cli"}},
	{id: "sublime", name: "Sublime Text", cliNames: []string{"subl"}},
	{id: "neovim", name: "Neovim", cliNames: []string{"nvim"}},
	{id: "jb-idea", name: "IntelliJ IDEA", cliNames: []string{"idea", "idea-ultimate", "idea-community"}},
	{id: "jb-pycharm", name: "PyCharm", cliNames: []string{"pycharm", "charm"}},
	{id: "jb-webstorm", name: "WebStorm", cliNames: []string{"webstorm"}},
	{id: "jb-goland", name: "GoLand", cliNames: []string{"goland"}},
	{id: "jb-rider", name: "Rider", cliNames: []string{"rider"}},
	{id: "jb-clion", name: "CLion", cliNames: []string{"clion"}},
	{id: "jb-phpstorm", name: "PhpStorm", cliNames: []string{"phpstorm"}},
	{id: "jb-rubymine", name: "RubyMine", cliNames: []string{"rubymine"}},
	{id: "jb-datagrip", name: "DataGrip", cliNames: []string{"datagrip"}},
}

func detectEditors() []Editor {
	out := []Editor{}
	seen := map[string]bool{}
	for _, ke := range unixEditors {
		exe := ""
		for _, cli := range ke.cliNames {
			if exe = lookPath(cli); exe != "" {
				break
			}
		}
		if exe == "" {
			exe = firstExisting(ke.paths...)
		}
		if exe == "" || seen[exe] {
			continue
		}
		seen[exe] = true
		out = append(out, Editor{ID: ke.id, Name: ke.name, Exec: exe, Args: ke.args, Kind: "editor", Detected: true})
	}

	// JetBrains Toolbox on Linux drops shims into ~/.local/share/JetBrains/Toolbox/scripts.
	if home, err := os.UserHomeDir(); err == nil {
		matches, _ := globSorted(filepath.Join(home, ".local/share/JetBrains/Toolbox/scripts", "*"))
		for _, m := range matches {
			base := filepath.Base(m)
			if seen[m] {
				continue
			}
			if info, err := os.Stat(m); err != nil || info.IsDir() {
				continue
			}
			seen[m] = true
			out = append(out, Editor{
				ID:       "jb-" + base,
				Name:     strings.ToUpper(base[:1]) + base[1:] + " (Toolbox)",
				Exec:     m,
				Kind:     "editor",
				Detected: true,
			})
		}
	}
	return out
}

func buildDetached(exe string, args []string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	proc.Detach(cmd)
	return cmd
}

// reveal selects the folder in the desktop file manager where that is
// supported, and falls back to just opening it.
func reveal(path string) error {
	if runtime.GOOS == "darwin" {
		return start(exec.Command("open", "-R", path))
	}
	for _, fm := range []string{"nautilus", "dolphin", "nemo", "thunar"} {
		if exe := lookPath(fm); exe != "" {
			args := []string{path}
			if fm == "dolphin" || fm == "nemo" {
				args = []string{"--select", path}
			}
			cmd := exec.Command(exe, args...)
			proc.Detach(cmd)
			return start(cmd)
		}
	}
	return openFolder(path)
}

func openFolder(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	exe := lookPath(opener)
	if exe == "" {
		return apperr.New(apperr.CodeFileManagerNone, "no file manager found (is xdg-open installed?)")
	}
	cmd := exec.Command(exe, path)
	proc.Detach(cmd)
	return start(cmd)
}

func openTerminal(path, prefer string) error {
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("open", "-a", "Terminal", path)
		proc.Detach(cmd)
		return start(cmd)
	}
	type candidate struct {
		id   string
		args []string
	}
	candidates := []candidate{
		{"gnome-terminal", []string{"--working-directory", path}},
		{"konsole", []string{"--workdir", path}},
		{"xfce4-terminal", []string{"--working-directory", path}},
		{"alacritty", []string{"--working-directory", path}},
		{"kitty", []string{"--directory", path}},
		{"wezterm", []string{"start", "--cwd", path}},
		{"xterm", []string{}},
	}
	if prefer != "" {
		for i, c := range candidates {
			if c.id == prefer {
				candidates[0], candidates[i] = candidates[i], candidates[0]
				break
			}
		}
	}
	for _, c := range candidates {
		exe := lookPath(c.id)
		if exe == "" {
			continue
		}
		cmd := exec.Command(exe, c.args...)
		cmd.Dir = path
		proc.Console(cmd)
		if err := cmd.Start(); err == nil {
			go func() { _ = cmd.Wait() }()
			return nil
		}
	}
	return apperr.New(apperr.CodeTerminalNotFound, "no terminal emulator found")
}

func openURL(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	exe := lookPath(opener)
	if exe == "" {
		return apperr.New(apperr.CodeFileManagerNone, "could not open a browser")
	}
	cmd := exec.Command(exe, url)
	proc.Detach(cmd)
	return start(cmd)
}
