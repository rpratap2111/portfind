// Package kill terminates a process the user has confirmed, after verifying
// that the PID still refers to the process they saw. For a port published by
// a Docker container it stops that container instead.
package kill

import (
	"fmt"
	"strings"

	"github.com/rpratap2111/portfind/internal/docker"
	"github.com/rpratap2111/portfind/internal/inspector"
)

// Target is the process the user confirmed killing, as it appeared in a scan.
type Target struct {
	PID     int
	Port    int
	Process string // display name, e.g. "node"; the container name for containers

	// ContainerID is set when the port is published by a Docker container.
	// Terminate then stops the container and never touches PID, which is
	// Docker's own process.
	ContainerID string
}

// TargetOf builds the kill target for a scanned entry.
func TargetOf(e inspector.PortEntry) Target {
	return Target{PID: e.PID, Port: e.Port, Process: e.Process, ContainerID: e.ContainerID}
}

// Terminate kills t after re-verifying it. Before terminating, the OS
// implementation confirms that the PID still runs an executable named
// t.Process and still listens on t.Port, so a PID recycled since the scan is
// never hit. It returns nil only once the process has actually exited.
func Terminate(ins inspector.PortInspector, t Target) error {
	if err := CheckAllowed(t); err != nil {
		return err
	}
	if t.ContainerID != "" {
		return docker.StopPublished(t.ContainerID, t.Port)
	}
	return terminate(ins, t)
}

// CheckAllowed reports why t may never be killed, or nil if it may be.
// Front-ends use it to avoid offering processes that would be refused.
func CheckAllowed(t Target) error {
	if t.ContainerID != "" {
		// The PID and critical-process rules are about host processes; a
		// container is stopped through Docker and may be named anything.
		return nil
	}
	if t.PID == 0 {
		return fmt.Errorf("refusing to kill PID 0: %s", pidZeroReason)
	}
	if t.PID <= 4 {
		return fmt.Errorf("refusing to kill PID %d: it is a core system process", t.PID)
	}
	if t.Process == "" {
		return fmt.Errorf("refusing to kill PID %d: its process name is unknown, so it cannot be verified", t.PID)
	}
	for _, c := range criticalProcesses {
		if strings.EqualFold(t.Process, c) {
			return fmt.Errorf("refusing to kill %s (PID %d): it is a critical system process", t.Process, t.PID)
		}
	}
	return nil
}
