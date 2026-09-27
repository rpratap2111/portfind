//go:build windows

package inspector

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// GetExtendedTcpTable is not wrapped by x/sys/windows, so bind it directly.
var (
	modIphlpapi             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modIphlpapi.NewProc("GetExtendedTcpTable")
)

const (
	afInet                   = 2
	afInet6                  = 23
	tcpTableOwnerPIDListener = 3 // TCP_TABLE_OWNER_PID_LISTENER
	maxTableReadAttempts     = 5

	pidSystemIdle = 0
	pidSystem     = 4
)

type windowsInspector struct {
	now func() time.Time
}

// New returns the PortInspector for the current OS.
func New() PortInspector {
	return &windowsInspector{now: time.Now}
}

func (w *windowsInspector) Scan() (ScanResult, error) {
	listeners, err := readAllListeners()
	if err != nil {
		return ScanResult{}, err
	}

	var res ScanResult
	procs, err := snapshotProcesses()
	if err != nil {
		// Name fallback and parent info are lost; keep going.
		res.Warnings = append(res.Warnings, fmt.Errorf("process snapshot unavailable: %w", err))
	}

	now := w.now()
	for _, l := range listeners {
		entry, err := resolveEntry(l, procs, now)
		if err != nil {
			res.Warnings = append(res.Warnings, err)
		}
		res.Entries = append(res.Entries, entry)
	}
	return res, nil
}

func (w *windowsInspector) IsListening(port, pid int) (bool, error) {
	listeners, err := readAllListeners()
	if err != nil {
		return false, err
	}
	return hasListener(listeners, port, pid), nil
}

func readAllListeners() ([]listener, error) {
	buf4, err := readTCPTable(afInet)
	if err != nil {
		return nil, fmt.Errorf("read IPv4 listener table: %w", err)
	}
	l4, err := parseTCP4Table(buf4)
	if err != nil {
		return nil, fmt.Errorf("parse IPv4 listener table: %w", err)
	}
	buf6, err := readTCPTable(afInet6)
	if err != nil {
		return nil, fmt.Errorf("read IPv6 listener table: %w", err)
	}
	l6, err := parseTCP6Table(buf6)
	if err != nil {
		return nil, fmt.Errorf("parse IPv6 listener table: %w", err)
	}
	return dedupeListeners(append(l4, l6...)), nil
}

// readTCPTable calls GetExtendedTcpTable for listening sockets of one address
// family, growing the buffer if the table changes size between calls.
func readTCPTable(family uint32) ([]byte, error) {
	if err := procGetExtendedTcpTable.Find(); err != nil {
		return nil, fmt.Errorf("locate GetExtendedTcpTable: %w", err)
	}
	size := uint32(16 * 1024)
	for attempt := 0; attempt < maxTableReadAttempts; attempt++ {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
			0, // bOrder: unsorted, we sort ourselves
			uintptr(family),
			tcpTableOwnerPIDListener,
			0,
		)
		switch errno := windows.Errno(r); errno {
		case windows.ERROR_SUCCESS:
			return buf, nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4 * 1024 // headroom in case sockets appear before the retry
		default:
			return nil, fmt.Errorf("GetExtendedTcpTable: %w", errno)
		}
	}
	return nil, fmt.Errorf("GetExtendedTcpTable: table kept growing after %d attempts", maxTableReadAttempts)
}

// resolveEntry fills in process details for one listener. On partial failure
// it still returns a usable entry (falling back to the snapshot name) along
// with an error describing what could not be resolved.
func resolveEntry(l listener, procs map[uint32]procInfo, now time.Time) (PortEntry, error) {
	e := PortEntry{Port: l.Port, PID: l.PID, AgeSeconds: -1}

	switch l.PID {
	case pidSystemIdle:
		e.Process = "System Idle Process"
		return e, nil
	case pidSystem:
		// The kernel "System" process starts at boot and cannot be opened.
		e.Process = "System"
		e.AgeSeconds = int64(windows.DurationSinceBoot().Seconds())
		return e, nil
	}

	self := procs[uint32(l.PID)]
	e.Process = DisplayName(self.exeFile)
	if parent, ok := procs[self.parentPID]; ok && self.parentPID != 0 {
		// Toolhelp parent PIDs can be stale if the parent exited and its PID
		// was reused. Accepted for now: the risk rules only escalate on the
		// parent name, so a stale match errs toward more confirmation.
		e.ParentPID = int(self.parentPID)
		e.ParentProcess = DisplayName(parent.exeFile)
	}

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(l.PID))
	if err != nil {
		return e, fmt.Errorf("port %d pid %d (%s): OpenProcess: %w", l.Port, l.PID, e.Process, err)
	}
	defer windows.CloseHandle(h)

	var errs []error
	if path, err := imagePath(h); err != nil {
		errs = append(errs, fmt.Errorf("QueryFullProcessImageName: %w", err))
	} else {
		e.Command = path
		e.Process = DisplayName(path)
	}
	if started, err := startTime(h); err != nil {
		errs = append(errs, fmt.Errorf("GetProcessTimes: %w", err))
	} else {
		e.AgeSeconds = int64(now.Sub(started).Seconds())
	}
	if len(errs) > 0 {
		return e, fmt.Errorf("port %d pid %d (%s): %w", l.Port, l.PID, e.Process, errors.Join(errs...))
	}
	return e, nil
}

func imagePath(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func startTime(h windows.Handle) (time.Time, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}

// procInfo is one process as seen by a Toolhelp snapshot.
type procInfo struct {
	exeFile   string
	parentPID uint32
}

// snapshotProcesses maps PID -> name and parent PID for every running process.
// Toolhelp works without special privileges, so it names processes that
// OpenProcess refuses (e.g. elevated services when running unelevated).
func snapshotProcesses() (map[uint32]procInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("CreateToolhelp32Snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	procs := make(map[uint32]procInfo)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	err = windows.Process32First(snap, &pe)
	for err == nil {
		procs[pe.ProcessID] = procInfo{
			exeFile:   windows.UTF16ToString(pe.ExeFile[:]),
			parentPID: pe.ParentProcessID,
		}
		err = windows.Process32Next(snap, &pe)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return procs, fmt.Errorf("Process32First/Next: %w", err)
	}
	return procs, nil
}
