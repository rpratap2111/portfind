// Package inspector enumerates processes that are listening on network ports.
//
// The PortInspector interface is OS-agnostic; each platform provides its own
// implementation behind a build tag (see windows.go, linux.go and darwin.go).
// Parsing and helper logic that does not touch OS APIs lives in untagged files
// so it can be unit-tested on any platform.
package inspector

import (
	"fmt"
	"strconv"
	"strings"
)

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

	// Set when the port is published by a Docker container. Process is then
	// the container's name (what the user sees and confirms), and killing
	// the entry stops that container rather than the Docker process (PID)
	// that holds the port for it.
	ContainerID string
	Image       string // e.g. "postgres:16"
}

// IsContainer reports whether the port belongs to a Docker container.
func (e PortEntry) IsContainer() bool { return e.ContainerID != "" }

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

// FormatAge renders an AgeSeconds value compactly, e.g. "45s", "12m",
// "2h15m", "3d4h". Negative values mean unknown and render as "?".
func FormatAge(sec int64) string {
	switch {
	case sec < 0:
		return "?"
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh%dm", sec/3600, sec%3600/60)
	default:
		return fmt.Sprintf("%dd%dh", sec/86400, sec%86400/3600)
	}
}

// uidName describes a user ID for messages, e.g. "root" or "uid 1001".
func uidName(uid int) string {
	if uid == 0 {
		return "root"
	}
	return "uid " + strconv.Itoa(uid)
}
