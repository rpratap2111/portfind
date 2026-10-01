//go:build darwin

package provenance

import (
	"errors"
	"fmt"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// readProcessParams reads the working directory (through libproc) and the
// command line (from the kern.procargs2 sysctl). Both are refused for another
// user's process when not root; Resolve then falls back to the executable's
// directory.
func readProcessParams(pid int) (processParams, error) {
	var p processParams
	var errs []error
	if cwd, err := inspector.ProcessCwd(pid); err != nil {
		errs = append(errs, fmt.Errorf("read cwd: %w", err))
	} else {
		p.Cwd = cwd
	}
	if cmd, err := inspector.ProcessCommandLine(pid); err != nil {
		errs = append(errs, fmt.Errorf("read command line: %w", err))
	} else {
		p.CommandLine = cmd
	}
	return p, errors.Join(errs...)
}
