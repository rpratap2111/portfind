//go:build !windows && !linux && !darwin

package provenance

import (
	"fmt"
	"runtime"
)

func readProcessParams(pid int) (processParams, error) {
	return processParams{}, fmt.Errorf("reading process parameters is not implemented for %s yet", runtime.GOOS)
}
