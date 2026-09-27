//go:build windows

package kill

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows"

	"portfind/internal/inspector"
)

const exitWaitMillis = 5000

func terminate(ins inspector.PortInspector, t Target) error {
	access := uint32(windows.PROCESS_TERMINATE | windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)
	h, err := windows.OpenProcess(access, false, uint32(t.PID))
	switch {
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		return fmt.Errorf("%s (PID %d) has already exited", t.Process, t.PID)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("OpenProcess %s (PID %d): %w (try running portfind as administrator)", t.Process, t.PID, err)
	case err != nil:
		return fmt.Errorf("OpenProcess %s (PID %d): %w", t.Process, t.PID, err)
	}
	// While this handle is open Windows cannot reuse the PID, so the checks
	// below still describe the same process when TerminateProcess runs.
	defer windows.CloseHandle(h)

	if err := verifyImage(h, t); err != nil {
		return err
	}
	listening, err := ins.IsListening(t.Port, t.PID)
	if err != nil {
		return fmt.Errorf("re-check port %d before killing: %w", t.Port, err)
	}
	if !listening {
		return fmt.Errorf("%s (PID %d) is no longer listening on :%d; not killed", t.Process, t.PID, t.Port)
	}

	if err := windows.TerminateProcess(h, 1); err != nil {
		return fmt.Errorf("TerminateProcess %s (PID %d): %w", t.Process, t.PID, err)
	}
	ev, err := windows.WaitForSingleObject(h, exitWaitMillis)
	if err != nil {
		return fmt.Errorf("wait for %s (PID %d) to exit: %w", t.Process, t.PID, err)
	}
	if ev != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("terminate sent, but %s (PID %d) was still running after %dms", t.Process, t.PID, exitWaitMillis)
	}
	return nil
}

// verifyImage checks that the process behind h is still the executable the
// user confirmed.
func verifyImage(h windows.Handle, t Target) error {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return fmt.Errorf("verify PID %d before killing: QueryFullProcessImageName: %w", t.PID, err)
	}
	got := inspector.DisplayName(windows.UTF16ToString(buf[:n]))
	if !strings.EqualFold(got, t.Process) {
		return fmt.Errorf("PID %d is now %q, not %q (PID was reused?); not killed", t.PID, got, t.Process)
	}
	return nil
}
