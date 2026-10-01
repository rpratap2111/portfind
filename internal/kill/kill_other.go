//go:build !windows && !linux && !darwin

package kill

import (
	"fmt"
	"runtime"

	"github.com/rpratap2111/portfind/internal/inspector"
)

func terminate(ins inspector.PortInspector, t Target) error {
	return fmt.Errorf("killing processes is not implemented for %s yet", runtime.GOOS)
}
