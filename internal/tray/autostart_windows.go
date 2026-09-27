//go:build windows

package tray

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// Start-with-Windows uses the per-user Run key, so it needs no admin rights
// and shows up (and can be disabled) in Task Manager's Startup apps.
// uninstall.ps1 deletes the same value.
const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "portfind-tray"

	// AutostartFlag marks a launch made by Windows at sign-in, so a second
	// instance exits quietly instead of showing "already running".
	AutostartFlag = "--autostart"
)

// autostartEnabled reports whether portfind-tray is registered to start at
// sign-in.
func autostartEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open %s: %w", runKeyPath, err)
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValueName)
	switch {
	case errors.Is(err, registry.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("read %s: %w", runValueName, err)
	}
	return true, nil
}

// setAutostart registers (or removes) this executable to start at sign-in.
func setAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s: %w", runKeyPath, err)
	}
	defer k.Close()

	if !on {
		if err := k.DeleteValue(runValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", runValueName, err)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate portfind-tray.exe: %w", err)
	}
	// Windows command-line quoting, not Go's %q, which would double the backslashes.
	if err := k.SetStringValue(runValueName, `"`+exe+`" `+AutostartFlag); err != nil {
		return fmt.Errorf("write %s: %w", runValueName, err)
	}
	return nil
}
