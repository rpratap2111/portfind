//go:build !windows && !linux && !darwin

package inspector

import (
	"fmt"
	"runtime"
)

type unsupportedInspector struct{}

// New returns the PortInspector for the current OS. Only Windows, Linux and
// macOS are implemented so far; other platforms get an inspector that reports
// so.
func New() PortInspector {
	return unsupportedInspector{}
}

func (unsupportedInspector) IsListening(port, pid int) (bool, error) {
	return false, fmt.Errorf("port inspection is not implemented for %s yet", runtime.GOOS)
}

func (unsupportedInspector) Scan() (ScanResult, error) {
	return ScanResult{}, fmt.Errorf("port inspection is not implemented for %s yet", runtime.GOOS)
}
