// Command portfind is an interactive terminal UI for finding and killing
// processes that are listening on network ports. See `portfind --help` for
// the other modes (--json, --update, --version).
package main

// Windows resources (icon, manifest) for local builds of portfind.exe; the
// release build regenerates them with version info (see .goreleaser.yaml).
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest cli --icon ../../assets/portfind.ico --product-name portfind --file-description "portfind terminal UI" --original-filename portfind.exe --copyright "MIT License"

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/scan"
	"github.com/rpratap2111/portfind/internal/tui"
	"github.com/rpratap2111/portfind/internal/update"
)

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	m, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "portfind: %v\nRun 'portfind --help' for usage.\n", err)
		os.Exit(2)
	}
	switch m {
	case modeHelp:
		printHelp(os.Stdout, displayVersion())
	case modeVersion:
		fmt.Println(displayVersion())
	case modeJSON:
		exitOn(runJSON())
	case modeUpdate:
		exitOn(runUpdate())
	default:
		exitOn(run())
	}
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "portfind:", err)
		os.Exit(1)
	}
}

// buildVersion prefers the release-stamped version, then the module version
// Go records for `go install ...@v0.1.0` builds, then "dev". It has no "v".
func buildVersion() string {
	if version != "dev" {
		return strings.TrimPrefix(version, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return version
}

// displayVersion is the version as users see it: "v1.1.1", or "dev".
func displayVersion() string {
	if v := buildVersion(); v != "dev" {
		return "v" + v
	}
	return "dev"
}

func run() error {
	// History is optional: if it can't be opened the TUI still works and
	// shows the error in its status line and warnings view.
	store, histErr := openHistory()
	if store != nil {
		defer store.Close()
	}

	m := tui.New(inspector.New(), store, histErr)
	// One background check per launch, for release builds; a failure (say,
	// offline) only shows in the warnings view. PORTFIND_NO_UPDATE_CHECK=1
	// turns it off.
	if v := buildVersion(); v != "dev" && os.Getenv("PORTFIND_NO_UPDATE_CHECK") == "" {
		if u, err := update.New(v); err == nil {
			m = m.WithUpdateCheck(u.Check)
		}
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// runJSON prints one scan as JSON. A scan failure exits non-zero with the
// error on stderr; per-process problems go in the report's "warnings".
func runJSON() error {
	res, err := scan.Run(inspector.New())
	if err != nil {
		return err
	}
	if err := writeJSON(os.Stdout, res); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

// runUpdate replaces this portfind (and, on Windows, the tray app next to it)
// with the latest release.
func runUpdate() error {
	u, err := update.New(buildVersion())
	if err != nil {
		return err
	}
	fmt.Println("Checking for updates...")
	res, err := u.Run()
	if err != nil {
		return err
	}
	if !res.Updated {
		fmt.Printf("portfind %s is already the latest version.\n", res.From)
		return nil
	}
	fmt.Printf("Updated portfind %s -> %s (%s).\n", res.From, res.To, strings.Join(res.Files, ", "))
	for _, f := range res.Files {
		if strings.HasPrefix(f, "portfind-tray") {
			fmt.Println("If the tray icon is running, quit it and open it again to use the new version.")
		}
	}
	fmt.Printf("What's new: %s/tag/%s\n", update.ReleasesURL, res.To)
	return nil
}

func openHistory() (*history.Store, error) {
	path, err := history.DefaultPath()
	if err != nil {
		return nil, err
	}
	return history.Open(path)
}
