// Package kill terminates a process the user has confirmed, after verifying
// that the PID still refers to the process they saw.
package kill

import (
	"fmt"
	"strings"

	"portfind/internal/inspector"
)

// Target is the process the user confirmed killing, as it appeared in a scan.
type Target struct {
	PID     int
	Port    int
	Process string // display name, e.g. "node"
}

// criticalProcesses are never killed, whatever the user confirms. Terminating
// any of these (possible when elevated) crashes or reboots Windows; svchost
// instances host core services such as RPC and should be stopped through the
// Services manager instead.
var criticalProcesses = []string{
	"system", "system idle process", "registry", "smss", "csrss",
	"wininit", "winlogon", "services", "lsass", "svchost",
}

// Terminate kills t after re-verifying it. Before terminating, the OS
// implementation confirms that the PID still runs an executable named
// t.Process and still listens on t.Port, so a PID recycled since the scan is
// never hit. It returns nil only once the process has actually exited.
func Terminate(ins inspector.PortInspector, t Target) error {
	if err := checkAllowed(t); err != nil {
		return err
	}
	return terminate(ins, t)
}

func checkAllowed(t Target) error {
	if t.PID <= 4 {
		return fmt.Errorf("refusing to kill PID %d: it is a kernel pseudo-process", t.PID)
	}
	if t.Process == "" {
		return fmt.Errorf("refusing to kill PID %d: its process name is unknown, so it cannot be verified", t.PID)
	}
	for _, c := range criticalProcesses {
		if strings.EqualFold(t.Process, c) {
			return fmt.Errorf("refusing to kill %s (PID %d): it is a critical Windows process", t.Process, t.PID)
		}
	}
	return nil
}
