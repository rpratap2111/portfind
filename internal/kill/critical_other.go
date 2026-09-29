//go:build !windows && !linux

package kill

var criticalProcesses []string

const pidZeroReason = "the owning process is unknown"
