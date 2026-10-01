//go:build darwin

package kill

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/rpratap2111/portfind/internal/inspector"
)

const (
	gracePeriod = 5 * time.Second // after SIGTERM, before SIGKILL
	killWait    = 2 * time.Second // after SIGKILL
)

// terminate asks the process to exit (SIGTERM, so dev servers can clean up)
// and forces it (SIGKILL) if it hasn't after gracePeriod.
//
// macOS has nothing like a Linux pidfd or a Windows process handle to pin a
// process with, so the process is identified by its PID plus its start time,
// which no later process with the same PID can share. That pair is checked
// again immediately before each signal, so a PID reused since the scan, or
// during the grace period, is never hit.
func terminate(ins inspector.PortInspector, t Target) error {
	started, _, err := inspector.ProcessStatus(t.PID)
	if err != nil {
		return fmt.Errorf("%s (PID %d) has already exited", t.Process, t.PID)
	}

	name, err := inspector.ProcessName(t.PID)
	if err != nil {
		return fmt.Errorf("verify PID %d before killing: %w", t.PID, err)
	}
	if !strings.EqualFold(name, t.Process) {
		return fmt.Errorf("PID %d is now %q, not %q (PID was reused?); not killed", t.PID, name, t.Process)
	}
	listening, err := ins.IsListening(t.Port, t.PID)
	if errors.Is(err, unix.EPERM) {
		return fmt.Errorf("re-check port %d before killing: %w (%s belongs to another user; try sudo)", t.Port, err, t.Process)
	}
	if err != nil {
		return fmt.Errorf("re-check port %d before killing: %w", t.Port, err)
	}
	if !listening {
		return fmt.Errorf("%s (PID %d) is no longer listening on :%d; not killed", t.Process, t.PID, t.Port)
	}

	if gone(t.PID, started) {
		return fmt.Errorf("%s (PID %d) exited before it could be killed", t.Process, t.PID)
	}
	if err := unix.Kill(t.PID, unix.SIGTERM); err != nil {
		return describe(t, err)
	}
	if exited(t.PID, started, gracePeriod) {
		return nil
	}
	if err := unix.Kill(t.PID, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		return describe(t, err)
	}
	if exited(t.PID, started, killWait) {
		return nil
	}
	return fmt.Errorf("sent SIGTERM and SIGKILL, but %s (PID %d) is still running", t.Process, t.PID)
}

func describe(t Target, err error) error {
	if errors.Is(err, unix.EPERM) {
		return fmt.Errorf("signal %s (PID %d): %w (it belongs to another user; try sudo)", t.Process, t.PID, err)
	}
	return fmt.Errorf("signal %s (PID %d): %w", t.Process, t.PID, err)
}

// exited waits up to timeout for the process that started at the given time
// to exit.
func exited(pid int, started time.Time, timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if gone(pid, started) {
			return true
		}
	}
	return gone(pid, started)
}

// gone reports whether the process is no longer running: the PID is free,
// belongs to a newer process, or is a zombie waiting to be reaped.
func gone(pid int, started time.Time) bool {
	now, zombie, err := inspector.ProcessStatus(pid)
	return err != nil || zombie || !now.Equal(started)
}
