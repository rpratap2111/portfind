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

	"github.com/rpratap2111/portfind/internal/tray"
)

func main() {
	err := tray.Run()
	switch {
	case errors.Is(err, tray.ErrAlreadyRunning):
		tray.ShowInfo("portfind is already running. Look for its icon in the notification area (you may need to click ^ to see hidden icons).")
	case err != nil:
		tray.ShowError(err)
		os.Exit(1)
	}
}
