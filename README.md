<div align="center">

# kisaf

### keep it simple as fuck project manager

**Keep every project folder on your machine in one place.**
See git status, reveal in your file manager, open in VS Code or JetBrains.

Single binary · zero dependencies · ~9 MB · no installation required

[Türkçe](README.tr.md) · [GPL-3.0](LICENSE)

</div>

---

## What it is for

You have dozens of project folders scattered across your disk and you keep
losing track of them. kisaf is a small local server plus a web interface that
turns that pile into a browsable list.

- **Card view** — one card per project: name, description, git status, tags and
  open task count at a glance. Buttons on the card open the project in your
  editor, file manager or terminal without leaving the list. With many projects,
  switch to the **compact list**.
- **Tasks** — a checklist per project. Add, tick off, delete, set a priority
  (low / normal / high), reorder, clear completed in one click. Progress shows
  on the card as a bar.
- **Add project** — pick a folder in the interface; it joins the list.
- **Scan folder** — point it at `D:\Projects` and it finds every git repository
  underneath and adds them in bulk.
- **Git status** — branch, number of changes, commits ahead/behind, as badges.
- **Commit history** — read commit messages in the interface; click one to
  expand its body.
- **Reveal** — opens the project folder in Explorer/Finder with it selected.
- **Open in editor** — VS Code, Cursor, Windsurf, Zed, Sublime, IntelliJ,
  Rider, PyCharm and more are detected automatically; each project can override
  the default.
- **Terminal** — Windows Terminal / PowerShell / your terminal, in that folder.
- **Notes and tags** — free-form notes, tags like `work` / `archive`.
- **Search** — across names, paths, tags, notes *and task text*.
- **README preview** and a **file tree** — look inside without opening it.

It never touches a file on disk. It only ever *points at* folders.

### How tasks work

The task list answers "where did I leave off?" with something you can tick,
rather than prose in a notes box. Each task has text, a done state and a
priority:

| Action | How |
|---|---|
| Add | Type in the box, pick a priority, press Enter |
| Complete / undo | The checkbox |
| Edit text | Click the text; it becomes editable (Enter saves, Escape cancels) |
| Change priority | Click the priority chip (normal → high → low) |
| Reorder | The ↑ ↓ buttons on the row |
| Delete | The ✕ on the row |
| Bulk cleanup | "Clear completed" |

Projects with open tasks can be filtered with **◧ Open tasks**, and open
high-priority work shows on the card as a red **urgent** badge.

---

## Languages

The interface ships in **English** and **Turkish**. It follows your browser's
language by default; Settings → Language pins it explicitly.

Adding a language is one file: copy the `en` block in
[`web/i18n.js`](web/i18n.js), translate the values, keep the keys, and add the
code to `LANGUAGES`. Server errors are localised too — the API sends a stable
code and its arguments, and the interface rebuilds the sentence, so a
translation covers them automatically. Pull requests welcome.

---

## Install

### Prebuilt binary (recommended)

1. Download `kisaf.exe` from [Releases](../../releases)
2. Double-click it — your browser opens on its own

For a permanent install (desktop shortcut, start with Windows, the `kisaf`
name):

```powershell
.\scripts\install-windows.ps1
```

To reach it from a phone or another computer:

```powershell
# in an administrator PowerShell
.\scripts\install-windows.ps1 -AllowNetwork
```

Uninstall with `.\scripts\install-windows.ps1 -Uninstall` — your project list
is kept.

### From source

You need only [Go 1.24+](https://go.dev/dl/). Nothing else.

```powershell
go build -o kisaf.exe .          # or:  .\scripts\build.ps1
```

```bash
go build -o kisaf .              # Linux / macOS
```

### Upgrading

Just run the new binary. If an older version is running it shuts itself down,
the new one takes over the port, and the browser shows the current interface.
Because the old process exits, its `.exe` also becomes deletable/overwritable
again.

If the same version is already running, no second server starts — you just get
a browser tab.

---

## Addresses: no "localhost" to type

Three ways in, all working at once:

| Address | Works from | How |
|---|---|---|
| `http://kisaf` | this machine (Windows) | LLMNR, built into Windows |
| `http://kisaf.local` | any device on the LAN | mDNS/Bonjour — Windows 10+, macOS, iOS, Android |
| `http://localhost` | this machine | always works, the fallback |

On port 80 there is no port suffix to remember. If the port is taken, kisaf
falls back to 7777 and prints the address to the console and log.

**To use it like an app:** press *Install* in your browser's address bar. kisaf
installs as a PWA and opens in its own window with no address bar. There is also
a system-tray icon: click to open, right-click for a menu.

---

## Remote access (homelab)

By default **only this machine** can reach it. Requests from anywhere else are
refused with an explanation.

To enable it, set a passphrase in the `token` field of
`%APPDATA%\kisaf\config.json` and restart:

```json
{
  "host": "kisaf",
  "port": 80,
  "bind": "0.0.0.0",
  "token": "a-long-passphrase-here",
  "allowedHosts": ["projects.home.lan"]
}
```

Visitors from the network now get a sign-in screen. The same binary runs on a
homelab server under systemd or Docker — `KISAF_TOKEN`, `KISAF_PORT`,
`KISAF_BIND`, `KISAF_HOST` and `KISAF_DATA_DIR` override the config file.

> Behind a reverse proxy (nginx/Caddy), add your domain to `allowedHosts` or the
> request is rejected.

---

## Security

This program can launch programs on your behalf, so a random web page must not
be able to drive it. Three layers stop that:

1. **Host allow-list** — defeats DNS rebinding. Even if an attacker's domain
   resolves to `127.0.0.1`, the request fails on the `Host` header.
2. **Origin check** — every mutating request is checked, so a page in another
   tab cannot add or delete projects via CSRF.
3. **Token** — required for every request that does not come from this machine.

Also:

- The API is never told *which command to run*. It receives an **editor id**,
  which is looked up in the list of detected programs.
- File endpoints cannot escape the project folder — symlinks are resolved
  before the check.

---

## Keyboard

| Key | Action |
|---|---|
| `/` | focus search |
| `Enter` (in search) | open the first result |
| `Ctrl` + `N` | new project |
| `Esc` | clear search / back to the list / close dialog |
| `Enter` (on a card) | open the project |

---

## Files

| Path | Contents |
|---|---|
| `%APPDATA%\kisaf\data.json` | your project list — plain JSON, hand-editable, backup-able |
| `%APPDATA%\kisaf\config.json` | process settings: port, name, token |
| `%APPDATA%\kisaf\kisaf.log` | the log (look here when something misbehaves) |

On Linux/macOS these live under `~/.config/kisaf`.

---

## Command line

```
kisaf [options]

  --port <n>       port to listen on (overrides config.json)
  --no-tray        do not start the system tray icon
  --no-browser     do not open a browser at startup
  --no-mdns        disable network discovery (the .local name)
  --version        print the version and exit
```

---

## How it works

```
kisaf.exe  ─┬─ HTTP server ──── embedded web interface (embed.FS)
            │                    REST API
            ├─ mDNS + LLMNR ──── answers for kisaf.local / kisaf
            ├─ tray icon ─────── Win32 via syscall, no cgo
            ├─ icons ─────────── drawn at runtime (internal/icon)
            └─ git ───────────── shells out to the git on your PATH
```

Design decisions:

- **Why a local server and not just a web page?** A page in a browser cannot
  open Explorer, launch VS Code, or browse folders on your disk. Those need a
  process on the machine. That same process leaves the door open for remote
  access for free.
- **Why Go, one binary?** No installer, no runtime, no `node_modules`. Copy the
  file, run it. Moving it to a homelab is also just copying one file.
- **Why JSON and not a database?** A few hundred rows do not justify carrying a
  SQLite driver. JSON stays readable, backup-able and fixable by hand.
- **Why the `git` command instead of a library?** A tenth of the code, and what
  it shows always matches what you see in your own terminal.
- **Why an interface with no build step?** About 2,000 lines of CSS and JS in
  total. Carrying webpack, npm and `node_modules` would be bigger than the
  problem it solves.

---

## Development

```bash
go test ./...                 # tests
go vet ./...                  # static checks
go run .                      # run it
```

Two things to know:

- The web interface is **embedded in the binary** (`embed.FS`). Change anything
  under `web/` and you must rebuild.
- There is not a single binary file in the repository. The icons are **drawn in
  code** in `internal/icon`; PNGs are produced on demand and cached, and the
  `.ico` Windows needs for the tray is written to the data folder on first run
  (~30 ms). To change the icon, edit the numbers in `internal/icon/icon.go` — no
  design tool involved.

---

## Contributing

Bug reports, translations and patches are all welcome. Please keep to what is
already here: no runtime dependencies, English source comments, and a test for
anything that could regress silently.

---

## License

[GPL-3.0](LICENSE) — free software. You may use, study, share and modify it;
if you distribute a modified version, it must stay under the same license.
