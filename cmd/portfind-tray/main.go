//go:build windows

// Command portfind-tray puts portfind in the Windows notification area: click
// the icon for a menu of listening ports and pick one to kill it.
//
// Build it as a GUI program so no console window appears:
//
//	go build -ldflags -H=windowsgui ./cmd/portfind-tray
package main

// Windows resources for local builds of portfind-tray.exe (see .goreleaser.yaml
// for release builds). The GUI manifest also turns on modern dialog styling
// and DPI awareness.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64,arm64 --manifest gui --icon ../../assets/portfind.ico --product-name portfind --file-description "portfind tray" --original-filename portfind-tray.exe --copyright "MIT License"

import (
	"errors"
	"os"
	"slices"

	"github.com/rpratap2111/portfind/internal/tray"
)

func main() {
	// Windows passes this when starting the tray at sign-in (see the Start
	// with Windows menu option).
	autostart := slices.Contains(os.Args[1:], tray.AutostartFlag)

	err := tray.Run()
	switch {
	case errors.Is(err, tray.ErrAlreadyRunning) && autostart:
		// Already started some other way; nothing to tell the user.
	case errors.Is(err, tray.ErrAlreadyRunning):
		tray.ShowInfo("portfind is already running. Look for its icon in the notification area (you may need to click ^ to see hidden icons).")
	case err != nil:
		tray.ShowError(err)
		os.Exit(1)
	}
}
