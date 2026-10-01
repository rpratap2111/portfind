//go:build !windows && !linux && !darwin

package kill

var criticalProcesses []string

const pidZeroReason = "the owning process is unknown"
