//go:build linux

package provenance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// readProcessParams reads the working directory and command line from /proc.
// For another user's process (when not root) the cwd link is unreadable;
// Resolve then falls back to the executable's directory.
func readProcessParams(pid int) (processParams, error) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	var p processParams
	var errs []error
	if cwd, err := os.Readlink(filepath.Join(dir, "cwd")); err != nil {
		errs = append(errs, fmt.Errorf("read cwd: %w", err))
	} else {
		p.Cwd = cwd
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cmdline")); err != nil {
		errs = append(errs, fmt.Errorf("read cmdline: %w", err))
	} else {
		p.CommandLine = inspector.ParseCmdline(data)
	}
	return p, errors.Join(errs...)
}
