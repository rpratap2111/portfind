//go:build !windows

package kill

import (
	"fmt"
	"runtime"

	"portfind/internal/inspector"
)

func terminate(ins inspector.PortInspector, t Target) error {
	return fmt.Errorf("killing processes is not implemented for %s yet", runtime.GOOS)
}
