# portfind

**Know what you're killing.** portfind finds the process holding a network port on Windows and lets you kill it, and before you do, it shows you *which project* that process belongs to, how long it has been running, and how risky killing it is.

```
╭──────────────────────────────────────────────────────────────────────────────────────────────╮
│ Search: 30█                          ⚡ :3000 killed 3× in 15m (node · my-app): something keeps… │
│                                                                                              │
│ ╭──────────────────────────────────────────────────────────────────────────────────────────╮ │
│ │ PORT   PID     PROJECT        PROCESS   AGE     RISK    COMMAND                          │ │
│ │ 3000   12240   my-app         node      2h15m   LOW     "C:\nodejs\node.exe" server.js   │ │
│ │ 3306   7840    -              mysqld    ?       HIGH                                     │ │
│ │ 5173   20412   portfolio      node      45s     LOW     node ...\vite\bin\vite.js        │ │
│ ╰──────────────────────────────────────────────────────────────────────────────────────────╯ │
│                                                                                              │
│ 27 ports · 3 matching · refreshed 14:03:12                                                   │
│ ↑/↓ nav  type to search  enter kill  tab history  ctrl+r refresh  ctrl+w warnings  esc quit  │
╰──────────────────────────────────────────────────────────────────────────────────────────────╯
```

## Install

Requires 64-bit Windows 10 or 11 (x64 or ARM64). No admin rights needed.

### One command (recommended)

Paste this into PowerShell:

```powershell
irm https://raw.githubusercontent.com/rpratap2111/portfind/main/install.ps1 | iex
```

Then run:

```powershell
portfind
```

The installer downloads the latest release for your CPU, **verifies its SHA-256 checksum**, puts `portfind.exe` in `%LOCALAPPDATA%\portfind\bin` and adds that folder to your user PATH. Terminal windows that were already open need a restart before they see `portfind`; the window you installed from works immediately.

<details>
<summary>Installer options</summary>

Set these before running the install command:

```powershell
$env:PORTFIND_VERSION = "v0.1.0"               # a specific release instead of the latest
$env:PORTFIND_INSTALL_DIR = "D:\tools\portfind" # somewhere other than %LOCALAPPDATA%\portfind\bin
$env:PORTFIND_NO_MODIFY_PATH = "1"              # don't touch PATH
```

</details>

### Manual download

1. Open the [latest release](https://github.com/rpratap2111/portfind/releases/latest) and download `portfind_windows_amd64.zip` (or `portfind_windows_arm64.zip` on Windows on ARM).
2. Extract it anywhere and run `portfind.exe`, either by double-clicking it or from a terminal.
3. Optional: add that folder to your PATH so you can type `portfind` from any terminal.

> **Windows SmartScreen:** the binaries aren't code-signed, so the first time you run a *browser-downloaded* `portfind.exe` Windows may show "Windows protected your PC". Click **More info → Run anyway**. The one-command installer downloads with PowerShell, which normally avoids this prompt.

### With Go

If you have Go 1.26 or newer:

```powershell
go install github.com/rpratap2111/portfind/cmd/portfind@latest
```

### Uninstall

Close portfind first, then:

```powershell
irm https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.ps1 | iex
```

This removes the executables and the PATH entry. Your kill history is kept. To delete it too, run this first in the same window:

```powershell
$env:PORTFIND_PURGE_HISTORY = "1"
```

Installed another way?
- **`go install`:** delete `%USERPROFILE%\go\bin\portfind.exe`.
- **Manual download:** delete the folder you extracted, and remove it from your PATH if you added it.

## Why portfind?

Most port killers answer one question: *which PID is on port 3000?* That's rarely enough. `node` on :3000 could be the app you forgot to stop, or a teammate's service you're about to take down. portfind adds the context you need to decide:

| | typical port killer | portfind |
|---|---|---|
| Port, PID, process name | ✓ | ✓ |
| **Project** (from `package.json`, `Cargo.toml`, `go.mod` or the git repo it was started in) | | ✓ |
| **Full command line** (`node server.js`, not just `node.exe`) | | ✓ |
| **Uptime** | | ✓ |
| **Risk tier** that decides how much confirmation a kill needs | | ✓ |
| Re-checks the PID right before killing, so a recycled PID is never hit | | ✓ |
| **Port-fight detection:** notices when something keeps respawning on a port | | ✓ |

## Usage

Run `portfind`. It opens on every listening TCP port, refreshes every 2 seconds and keeps your search and selection while it does.

| Key | Action |
|---|---|
| *type anything* | Filter by port, process or project (fuzzy: `dwa` matches `demo-web-app`; `node 30` needs both) |
| `↑` `↓` | Move the selection |
| `Enter` | Kill the selected process (asks for confirmation) |
| `Tab` | History of processes that left their ports |
| `Ctrl+R` | Refresh now |
| `Ctrl+W` | Show warnings (e.g. processes Windows wouldn't let portfind inspect) |
| `Esc` | Back out of a view or dialog; quits from the port list |
| `Ctrl+C` | Quit |

Every key you can type goes into the search box, so all commands live on non-typing keys.

### Risk tiers

| Tier | What | To kill |
|---|---|---|
| **LOW** | Dev runtimes: `node`, `python`, `ruby`, `java`, `dlv`, anything started by `go run` | Press `y` in a `[y/N]` prompt |
| **MEDIUM** | Anything unrecognized, and Windows services | Type the process name |
| **HIGH** | Databases (`postgres`, `mysql`, `mongod`, `redis-server`, `sqlservr`) and anything started over SSH | Type the process name |

Before killing, portfind holds the process open (so Windows can't reuse its PID), checks that it's still the same executable and still listening on that port, and only then terminates it and waits for it to exit. Core Windows processes such as `lsass`, `csrss`, `wininit`, `services` and `svchost` are always refused, since killing them crashes or reboots Windows.

### Port fights and history

portfind logs every process that leaves a port, and whether portfind killed it. If you've killed the same port **3 times in 15 minutes**, a hint appears next to the search box: usually `nodemon`, a supervisor or an auto-restarting service is bringing it back, and killing it again won't help. Press `Tab` to browse the history.

History is stored in `%LOCALAPPDATA%\portfind\history.db` (SQLite).

## Limitations

- **Windows only**, for now. The code is laid out for Linux and macOS support later.
- **Services and elevated processes:** without admin rights Windows won't let portfind read their details (age, command line, project) or kill them. They still appear, with a warning under `Ctrl+W`. Run portfind as administrator to manage them.
- **Project detection** uses the process's *current* working directory, falling back to the folder of its executable. A process that changed directory after starting may be attributed to the wrong project.
- TCP listeners only; UDP isn't shown.

## Build from source

```powershell
git clone https://github.com/rpratap2111/portfind
cd portfind
go build ./cmd/portfind
.\portfind.exe
```

Pure Go, no cgo: the SQLite driver is `modernc.org/sqlite`, and Windows is queried directly through its APIs (`GetExtendedTcpTable`, `NtQueryInformationProcess`, …) rather than by parsing `netstat`.

```powershell
go test ./...
```

### Releasing

Push a version tag. GitHub Actions runs the tests, then [GoReleaser](https://goreleaser.com) builds the Windows zips and `checksums.txt` and publishes the release:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

## License

[MIT](LICENSE)
