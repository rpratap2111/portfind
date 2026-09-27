// Command portfind is an interactive terminal UI for finding and killing
// processes that are listening on network ports.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/tui"
)

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("portfind", buildVersion())
		return
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "portfind:", err)
		os.Exit(1)
	}
}

// buildVersion prefers the release-stamped version, then the module version
// Go records for `go install ...@v0.1.0` builds, then "dev".
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return version
}

func run() error {
	// History is optional: if it can't be opened the TUI still works and
	// shows the error in its status line and warnings view.
	store, histErr := openHistory()
	if store != nil {
		defer store.Close()
	}

	m := tui.New(inspector.New(), store, histErr)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func openHistory() (*history.Store, error) {
	path, err := history.DefaultPath()
	if err != nil {
		return nil, err
	}
	return history.Open(path)
}
