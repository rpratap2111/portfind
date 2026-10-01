# Portfind

> **Know what you're killing.** A smart, cross-platform port and process manager with project context, risk tiers, and zero accidental downtime.

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-0078D6?style=for-the-badge)](https://github.com/rpratap2111/portfind/releases)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)

### Topic Tags
[![Topic: Port Management](https://img.shields.io/badge/Topic-Port_Management-0052CC?style=for-the-badge&logo=target&logoColor=white)](#)
[![DevTools](https://img.shields.io/badge/Ecosystem-Developer_Tools-FF6F00?style=for-the-badge&logo=visualstudiocode&logoColor=white)](#)

### Tech Stack
[![Go / Golang](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![SQLite](https://img.shields.io/badge/SQLite-modernc.org%2Fsqlite-003B57?style=for-the-badge&logo=sqlite&logoColor=white)](https://modernc.org/sqlite)
[![Windows API](https://img.shields.io/badge/Windows_API-GetExtendedTcpTable-0078D6?style=for-the-badge&logo=windows&logoColor=white)](#)
[![Linux pidfd](https://img.shields.io/badge/Linux-pidfd_%26_procfs-FCC624?style=for-the-badge&logo=linux&logoColor=black)](#)
[![macOS libproc](https://img.shields.io/badge/macOS-libproc_%26_sysctl-000000?style=for-the-badge&logo=apple&logoColor=white)](#)
[![PowerShell & Bash](https://img.shields.io/badge/Scripts-PowerShell_%26_Bash-5391FE?style=for-the-badge&logo=powershell&logoColor=white)](#)

</div>

---

## Brief Summary

**portfind** finds the process holding a network port on Windows, Linux or macOS and lets you kill it safely. But unlike traditional port killers, before you press kill, it reveals **which project** that process belongs to, its full command line, how long it has been running, and its safety risk level.

It comes as:
- An interactive **Terminal UI** (`portfind`) on Windows, Linux and macOS.
- A discreet **Notification-Area Tray Icon** (`portfind-tray`) on Windows with quick actions.
- A zero-overhead **JSON CLI mode** (`portfind --json`) for automated developer scripts.

### Core Highlights:
- **Project Awareness:** Scans `package.json`, `Cargo.toml`, `go.mod`, or Git repositories to tell you which codebase spawned the process.
- **Risk Tiers:** Categorizes processes as **LOW**, **MEDIUM**, or **HIGH** risk so you don't accidentally shut down a database or an active SSH session.
- **Safe Termination:** Pins the process (`pidfd` on Linux / process handles on Windows / PID plus start time on macOS) before stopping it, preventing accidental kills if a PID is recycled.
- **Port-Fight Detection:** Detects when a supervisor (like `nodemon`) keeps reviving a process on the same port and warns you before you enter an endless kill loop.
- **Persistent History:** SQLite-backed audit trail of all freed ports and terminated processes.

---

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

---

## Install

### Windows

Requires 64-bit Windows 10 or 11 (x64 or ARM64). No admin rights needed.

#### One command (recommended)

Paste this into PowerShell:

```powershell
irm https://raw.githubusercontent.com/rpratap2111/portfind/main/install.ps1 | iex
```

Then run:

```powershell
portfind
```

The installer downloads the latest release for your CPU, **verifies its SHA-256 checksum**, puts `portfind.exe` and `portfind-tray.exe` in `%LOCALAPPDATA%\portfind\bin` and adds that folder to your user PATH. Terminal windows that were already open need a restart before they see `portfind`; the window you installed from works immediately.

<details>
<summary>Installer options</summary>

Set these before running the install command:

```powershell
$env:PORTFIND_VERSION = "v0.1.0"               # a specific release instead of the latest
$env:PORTFIND_INSTALL_DIR = "D:\tools\portfind" # somewhere other than %LOCALAPPDATA%\portfind\bin
$env:PORTFIND_NO_MODIFY_PATH = "1"              # don't touch PATH
```

</details>

#### Manual download

1. Open the [latest release](https://github.com/rpratap2111/portfind/releases/latest) and download `portfind_windows_amd64.zip` (or `portfind_windows_arm64.zip` on Windows on ARM).
2. Extract it anywhere. Run `portfind.exe` for the terminal UI or `portfind-tray.exe` for the tray icon, and keep the two files in the same folder.
3. Optional: add that folder to your PATH so you can type `portfind` from any terminal.

> **Windows SmartScreen:** the binaries aren't code-signed, so the first time you run a *browser-downloaded* `portfind.exe` Windows may show "Windows protected your PC". Click **More info → Run anyway**. The one-command installer downloads with PowerShell, which normally avoids this prompt.

#### With Go

If you have Go 1.26 or newer:

```powershell
go install github.com/rpratap2111/portfind/cmd/portfind@latest
```

#### Updating

```powershell
portfind --update
```

This checks GitHub for a newer release and, if there is one, downloads it, verifies its checksum, and replaces both `portfind.exe` and `portfind-tray.exe`. If the tray icon is running, quit it and open it again afterwards. Or let the tray do it: when an update is out, its menu shows **Restart to update**. Re-running the install command also updates.

#### Uninstall

Close portfind first, including the tray icon (click it → **Quit**), then:

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

---

### Linux

Requires 64-bit Linux (x86_64 or arm64); any distribution. No root needed.

```sh
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | sh
```

The installer downloads the latest release for your CPU, **verifies its SHA-256 checksum**, puts `portfind` in `~/.local/bin` and, if that folder isn't on your PATH yet, adds it in `~/.bashrc` (and `~/.zshrc` if you use zsh). Open a new terminal and run `portfind`, or paste the `export PATH=…` line the installer prints to use it straight away. It works under `sudo` too, which lets it see and stop other users' processes.

<details>
<summary>Updating, installer options and uninstall</summary>

```sh
# Update to the latest release (verifies the checksum first):
portfind --update

# A specific release, or another install directory:
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_VERSION=v1.1.0 sh
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_INSTALL_DIR=~/bin sh

# Don't touch ~/.bashrc / ~/.zshrc:
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_NO_MODIFY_PATH=1 sh

# Uninstall: removes the binary and the PATH lines; keeps your history
# (add PORTFIND_PURGE_HISTORY=1 before `sh` to delete it too):
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.sh | sh
```

Or download `portfind_linux_amd64.tar.gz` / `portfind_linux_arm64.tar.gz` from the [latest release](https://github.com/rpratap2111/portfind/releases/latest) and extract `portfind` anywhere on your PATH, or use `go install` (see *With Go* above; it works on Linux too).

</details>

---

### macOS

Requires macOS 12 (Monterey) or later, on Apple silicon or Intel. No root needed.

```sh
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | sh
```

It is the same installer as on Linux: it downloads the latest release for your CPU, **verifies its SHA-256 checksum** and puts `portfind` in `~/.local/bin`. If that folder isn't on your PATH yet, it adds it in `~/.zshrc` (and `~/.bash_profile` if you use bash). Open a new terminal and run `portfind`, or paste the `export PATH=…` line the installer prints to use it straight away. It works under `sudo` too, which lets it see and stop other users' processes.

Updating, the installer options and uninstalling work exactly as on Linux (see the block above). The uninstaller keeps your history in `~/Library/Caches/portfind` unless you set `PORTFIND_PURGE_HISTORY=1`.

Or download `portfind_darwin_arm64.tar.gz` (Apple silicon) / `portfind_darwin_amd64.tar.gz` (Intel) from the [latest release](https://github.com/rpratap2111/portfind/releases/latest) and extract `portfind` anywhere on your PATH, or use `go install` (see *With Go* above).

> **Gatekeeper:** the binaries aren't notarized, so macOS blocks a *browser-downloaded* `portfind` the first time ("Apple could not verify…"). Clear the download flag with `xattr -d com.apple.quarantine ./portfind`. The installer downloads with `curl`, which doesn't set the flag, so it avoids this.

---

## Why portfind?

Port killers such as [pik](https://github.com/jacek-kurlit/pik), pview, PortSlayer and portndock are built around one question: *which process is on port 3000?* They show you OS-level facts (PID, process name) and let you kill it. That's rarely enough on its own. `node` on :3000 could be the app you forgot to stop, or a teammate's service you're about to take down. portfind is built around a different question, **"what exactly am I about to kill?"**, and adds the context you need to answer it:

| Feature | Typical port killer | **portfind** |
|---|:---:|:---:|
| Port, PID, process name | ✓ | **✓** |
| **Project** (from `package.json`, `Cargo.toml`, `go.mod` or the git repo it was started in) | ✗ | **✓** |
| **Full command line** (`node server.js`, not just `node.exe`) | ✗ | **✓** |
| **Uptime** | ✗ | **✓** |
| **Risk tier** that decides how much confirmation a kill needs | ✗ | **✓** |
| Re-checks PID right before killing (no recycled PID accidents) | ✗ | **✓** |
| **Port-fight detection:** notices when something keeps respawning on a port | ✗ | **✓** |
| **Kill history tracking** (SQLite persistent storage) | ✗ | **✓** |
| **Windows Notification Tray** with 1-click safe kill | ✗ | **✓** |

---

## Usage

| Command | What it does |
|---|---|
| `portfind` | Open the interactive terminal UI |
| `portfind --json` | Print listening ports as JSON and exit, for scripts ([details](#scripting-with---json)) |
| `portfind --update` | Update portfind to the latest release |
| `portfind --version` | Print the version, e.g. `v1.2.0` (also `-v`) |
| `portfind --help` | Show commands and keys (also `-h`) |

### The terminal UI

Run `portfind`. It opens on every listening TCP port, refreshes every 2 seconds and keeps your search and selection while it does.

| Key | Action |
|---|---|
| *type anything* | Filter by port, process or project (fuzzy: `dwa` matches `demo-web-app`; `node 30` needs both) |
| `↑` `↓` | Move the selection |
| `Enter` | Kill the selected process (asks for confirmation) |
| `Tab` | History of processes that left their ports |
| `Ctrl+R` | Refresh now |
| `Ctrl+W` | Show warnings (e.g. processes the OS wouldn't let portfind inspect) |
| `Esc` | Back out of a view or dialog; quits from the port list |
| `Ctrl+C` | Quit |

Every key you can type goes into the search box, so all commands live on non-typing keys.

---

### Scripting with `--json`

`portfind --json` prints every listening port as JSON and exits, with no UI and no history. Every field is always present, and anything portfind couldn't determine is `null`:

```json
{
  "ports": [
    {
      "port": 8899,
      "pid": 17012,
      "process": "python",
      "project": "git-only-repo",
      "age_seconds": 2,
      "risk": "LOW",
      "command": "\"C:\\...\\python.exe\" -m http.server 8899",
      "parent_pid": 5920,
      "parent_process": "pwsh"
    }
  ],
  "warnings": ["port 135 pid 1984 (svchost): OpenProcess: Access is denied."]
}
```

`warnings` lists processes the OS wouldn't let portfind fully inspect; their entries still appear, with `null`s. If the scan itself fails, portfind exits with code 1 and prints the error to stderr.

#### PowerShell:

```powershell
# What's on port 3000?
(portfind --json | ConvertFrom-Json).ports | Where-Object port -eq 3000

# Every port held by one of your projects
(portfind --json | ConvertFrom-Json).ports | Where-Object project | Format-Table port, process, project
```

#### jq:

```sh
portfind --json | jq '.ports[] | select(.risk == "LOW") | {port, process, project}'
```


---

### Risk Tiers

| Tier | What | To kill |
|---|---|---|
| **LOW** | Dev runtimes: `node`, `python`, `ruby`, `java`, `dlv`, anything started by `go run` | Press `y` in a `[y/N]` prompt |
| **MEDIUM** | Anything unrecognized, and system services | Type the process name |
| **HIGH** | Databases (`postgres`, `mysql`, `mongod`, `redis-server`, `sqlservr`) and anything started over SSH | Type the process name |

Before killing, portfind pins the process so a reused PID can't be hit (a process handle on Windows, a pidfd on Linux, the PID together with its start time on macOS), checks that it's still the same executable and still listening on that port, and only then stops it and waits for it to exit.

- **Windows:** the process is terminated. Core processes such as `lsass`, `csrss`, `wininit`, `services` and `svchost` are always refused, since killing them crashes or reboots Windows.
- **Linux:** the process gets `SIGTERM` so it can shut down cleanly, then `SIGKILL` if it's still running after 5 seconds. `systemd`, `init`, `sshd` (you could lock yourself out of a remote machine) and `systemd-resolved` (DNS) are always refused; use `systemctl` for services.
- **macOS:** the same `SIGTERM`, then `SIGKILL` after 5 seconds. `launchd`, `kernel_task`, `WindowServer` and `loginwindow` (killing either ends your login session), `sshd` and `mDNSResponder` (DNS) are always refused; use `launchctl` for services.

---

### Tray Icon (Windows)

Press `Win`, search **portfind** and open it (or run `portfind-tray`). An icon appears in the notification area. On Windows 11 it may start in the `^` overflow; drag it onto the taskbar to keep it visible. Click the icon and portfind rescans, then lists the ports you're most likely to want back, dev servers first. Each port opens a submenu with its details and a separate **Kill** item, so a stray click on a port never kills anything:

```
portfind v1.3.0 · 40 listening ports
──────────────────────────────────────────────
python — :8899 (git-only-repo)        LOW    ▸ ┌──────────────────────────────────┐
mystery-daemon — :9300 (fixtures)     MEDIUM ▸ │ PID 12528 · running 4s · LOW risk │
…                                              │ Project: git-only-repo            │
20 system or elevated ports not shown          │ "…\python.exe" -m http.server 8899│
…and 8 more (Open Terminal UI to see all)      │ Kill python                       │
──────────────────────────────────────────────  └──────────────────────────────────┘
portfind v1.4.0 is available                     (only when there's an update)
Restart to update
Open Terminal UI
✓ Start with Windows
Quit
```

- **Kill** on a LOW process kills it straight away, and a notification confirms it.
- **Kill…** on a MEDIUM or HIGH process asks first, in a dialog showing the project, PID, uptime and command. **No** is the default button.
- Ports that can never be killed (core Windows processes) or that Windows won't let you touch without admin rights aren't listed. The menu says how many were left out.
- If a port-fight is in progress, a `⚡` line says so.
- **Open Terminal UI** opens `portfind` in a new terminal window.
- **Restart to update** appears when a newer release is out. The tray checks GitHub shortly after it starts, every 6 hours, and when you open the menu (at most once every 15 minutes), and lets you know once with a notification. One click downloads the new version, verifies its checksum, replaces both programs and restarts the tray.
- **Start with Windows** starts the tray icon when you sign in. Click it again to turn it off, or use Task Manager's Startup apps. It needs no admin rights, and the uninstaller turns it off.

---

### Port Fights and History

portfind logs every process that leaves a port, and whether portfind killed it. Kills made from the tray are logged too. If you've killed the same port **3 times in 15 minutes**, a hint appears next to the search box: usually `nodemon`, a supervisor or an auto-restarting service is bringing it back, and killing it again won't help. Press `Tab` to browse the history.

History is stored in SQLite, at:
- **Windows:** `%LOCALAPPDATA%\portfind\history.db`
- **Linux:** `~/.cache/portfind/history.db`
- **macOS:** `~/Library/Caches/portfind/history.db`

---

## Limitations

- **Tray icon:** Windows-only. On Linux and macOS use the terminal UI.
- **Other users' and elevated processes:**
  - On Windows, without admin rights portfind can't read the details (age, command line, project) of services and elevated processes, or kill them. They still appear, with a warning under `Ctrl+W`. Run portfind as administrator to manage them.
  - On Linux and macOS, a normal user can't see which of *another user's* processes owns a port. The port still appears, as `(unknown)`, with a warning naming the owner (for example root). Run `sudo portfind` to see and manage those.
- **Project detection** uses the process's *current* working directory, falling back to the folder of its executable. A process that changed directory after starting may be attributed to the wrong project.
- **TCP listeners only;** UDP isn't shown.

---

## Build From Source

```powershell
git clone https://github.com/rpratap2111/portfind
cd portfind
go build ./cmd/portfind
.\portfind.exe
go build -ldflags -H=windowsgui ./cmd/portfind-tray  # GUI build: no console window
.\portfind-tray.exe
```

On Linux and macOS:

```sh
go build ./cmd/portfind
./portfind
```

Pure Go, no cgo: the SQLite driver is `modernc.org/sqlite`, and each OS is queried directly through its own interfaces rather than by parsing `netstat` or `lsof`: `GetExtendedTcpTable`, `NtQueryInformationProcess`, … on Windows, `/proc` on Linux, and `libproc` plus `sysctl` on macOS. So every target cross-compiles from any machine, e.g. `GOOS=darwin GOARCH=arm64 go build ./cmd/portfind`.

```powershell
go test ./...
```

### Releasing

Push a version tag. GitHub Actions runs the tests, then [GoReleaser](https://goreleaser.com) builds the Windows zips, the Linux and macOS tarballs and `checksums.txt`, and publishes the release:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

---

## License

Distributed under the [MIT](LICENSE) License.
