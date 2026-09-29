//go:build linux

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
// A pidfd pins the process like a Windows handle does: signals sent through
// it can only reach that exact process, even if the PID is reused meanwhile.
// Kernels older than 5.3 lack pidfds; there portfind falls back to kill(2)
// and relies on the name and port checks just before signalling.
func terminate(ins inspector.PortInspector, t Target) error {
	fd, err := unix.PidfdOpen(t.PID, 0)
	switch {
	case errors.Is(err, unix.ESRCH):
		return fmt.Errorf("%s (PID %d) has already exited", t.Process, t.PID)
	case errors.Is(err, unix.ENOSYS):
		fd = -1
	case err != nil:
		return fmt.Errorf("pidfd_open %s (PID %d): %w", t.Process, t.PID, err)
	}
	if fd >= 0 {
		defer unix.Close(fd)
	}

	name, err := inspector.ProcessName(t.PID)
	if err != nil {
		return fmt.Errorf("verify PID %d before killing: %w", t.PID, err)
	}
	if !strings.EqualFold(name, t.Process) {
		return fmt.Errorf("PID %d is now %q, not %q (PID was reused?); not killed", t.PID, name, t.Process)
	}
	listening, err := ins.IsListening(t.Port, t.PID)
	if err != nil {
		return fmt.Errorf("re-check port %d before killing: %w", t.Port, err)
	}
	if !listening {
		return fmt.Errorf("%s (PID %d) is no longer listening on :%d; not killed", t.Process, t.PID, t.Port)
	}

	if err := signal(fd, t.PID, unix.SIGTERM); err != nil {
		return describe(t, err)
	}
	if exited(fd, t.PID, gracePeriod) {
		return nil
	}
	if err := signal(fd, t.PID, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		return describe(t, err)
	}
	if exited(fd, t.PID, killWait) {
		return nil
	}
	return fmt.Errorf("sent SIGTERM and SIGKILL, but %s (PID %d) is still running", t.Process, t.PID)
}

func signal(fd, pid int, sig unix.Signal) error {
	if fd >= 0 {
		return unix.PidfdSendSignal(fd, sig, nil, 0)
	}
	return unix.Kill(pid, sig)
}

func describe(t Target, err error) error {
	if errors.Is(err, unix.EPERM) {
		return fmt.Errorf("signal %s (PID %d): %w (it belongs to another user; try sudo)", t.Process, t.PID, err)
	}
	return fmt.Errorf("signal %s (PID %d): %w", t.Process, t.PID, err)
}

// exited waits up to timeout for the process to exit. A pidfd becomes
// readable when its process exits, even before the parent reaps it; without
// one, a zombie counts as exited.
func exited(fd, pid int, timeout time.Duration) bool {
	if fd >= 0 {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		deadline := time.Now().Add(timeout)
		for {
			n, err := unix.Poll(fds, int(time.Until(deadline).Milliseconds()))
			if err == unix.EINTR && time.Now().Before(deadline) {
				continue
			}
			return err == nil && n > 0
		}
	}
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if gone(pid) {
			return true
		}
	}
	return gone(pid)
}

func gone(pid int) bool {
	if err := unix.Kill(pid, 0); errors.Is(err, unix.ESRCH) {
		return true
	}
	state, err := inspector.ProcessState(pid)
	return err != nil || state == 'Z'
}
