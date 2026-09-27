// Command portfind is an interactive terminal UI for finding and killing
// processes that are listening on network ports.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"portfind/internal/history"
	"portfind/internal/inspector"
	"portfind/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "portfind:", err)
		os.Exit(1)
	}
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
