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
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/rpratap2111/portfind/internal/tray"
)

// version is set at release time via -ldflags "-X main.version=...".
var version = "dev"

// updatedFromFlag is passed by the previous instance after "Restart to
// update", so the new one can confirm the update.
const updatedFromFlag = "--updated-from"

func main() {
	args := os.Args[1:]
	// Windows passes this when starting the tray at sign-in (see the Start
	// with Windows menu option).
	autostart := slices.Contains(args, tray.AutostartFlag)

	err := tray.Run(tray.Options{
		Version:     strings.TrimPrefix(version, "v"),
		UpdatedFrom: flagValue(args, updatedFromFlag),
	})
	var restart *tray.RestartError
	switch {
	case errors.As(err, &restart):
		// Run has released the icon and the single-instance lock; start the
		// newly installed tray and let this (old) one exit.
		if err := exec.Command(restart.Path, restart.Args...).Start(); err != nil {
			tray.ShowError(fmt.Errorf("portfind was updated, but the new version didn't start (%v). Open portfind from the Start menu.", err))
			os.Exit(1)
		}
	case errors.Is(err, tray.ErrAlreadyRunning) && autostart:
		// Already started some other way; nothing to tell the user.
	case errors.Is(err, tray.ErrAlreadyRunning):
		tray.ShowInfo("portfind is already running. Look for its icon in the notification area (you may need to click ^ to see hidden icons).")
	case err != nil:
		tray.ShowError(err)
		os.Exit(1)
	}
}

// flagValue returns the value after name in args ("--name value" or
// "--name=value"), or "".
func flagValue(args []string, name string) string {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
