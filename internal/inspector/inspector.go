// Package inspector enumerates processes that are listening on network ports.
//
// The PortInspector interface is OS-agnostic; each platform provides its own
// implementation behind a build tag (see windows.go). Parsing and helper logic
// that does not touch OS APIs lives in untagged files so it can be unit-tested
// on any platform.
package inspector

import "strings"

// PortEntry describes one process listening on one port. It is the shared data
// model used by the CLI, TUI and tray front-ends.
type PortEntry struct {
	Port        int
	PID         int
	Process     string // executable base name without ".exe", e.g. "node"
	ProjectName string // resolved via provenance, empty if not found
	AgeSeconds  int64  // seconds since process start; -1 if unknown
	RiskTier    string // "LOW" | "MEDIUM" | "HIGH"
	Command     string // full command line if provenance could read it, else executable path

	ParentPID     int    // 0 if unknown
	ParentProcess string // parent's display name, e.g. "sshd"; empty if unknown
}

// ScanResult is the output of a scan. Warnings holds non-fatal, per-process
// failures (e.g. access denied when opening an elevated process). The affected
// entries are still returned with whatever could be resolved; callers should
// surface Warnings to the user rather than discard them.
type ScanResult struct {
	Entries  []PortEntry
	Warnings []error
}

// PortInspector lists processes that own listening sockets.
type PortInspector interface {
	// Scan returns one entry per unique (port, PID) pair, sorted by port then
	// PID. A non-nil error means the scan as a whole failed.
	Scan() (ScanResult, error)

	// IsListening reports whether pid currently owns a listening socket on
	// port. It is a cheap re-check used right before killing a process.
	IsListening(port, pid int) (bool, error)
}

// DisplayName turns an executable path or file name into the short process
// name shown to users: base name with a trailing ".exe" removed. Both slash
// styles are handled so this behaves identically on every OS.
func DisplayName(exe string) string {
	if i := strings.LastIndexAny(exe, `\/`); i >= 0 {
		exe = exe[i+1:]
	}
	if len(exe) > 4 && strings.EqualFold(exe[len(exe)-4:], ".exe") {
		exe = exe[:len(exe)-4]
	}
	return exe
}
