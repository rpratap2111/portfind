//go:build linux

package inspector

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// clockTicks is USER_HZ, the unit of /proc/<pid>/stat's starttime. The kernel
// fixes it at 100 on every mainstream architecture; reading it properly needs
// sysconf(_SC_CLK_TCK), i.e. cgo, which portfind avoids.
const clockTicks = 100

type linuxInspector struct {
	proc string // "/proc"; a parameter so tests could point elsewhere
	now  func() time.Time
}

// New returns the PortInspector for the current OS.
func New() PortInspector {
	return &linuxInspector{proc: "/proc", now: time.Now}
}

func (li *linuxInspector) Scan() (ScanResult, error) {
	socks, err := li.listeningSockets()
	if err != nil {
		return ScanResult{}, err
	}
	var res ScanResult
	owners := li.socketOwners()

	// Sockets whose owner we can't see (another user's process, when not
	// root) are still listed, with PID 0, so the user knows the port is taken.
	var listeners []listener
	hiddenUID := map[int]int{} // port -> uid of the owner we can't see
	for _, s := range socks {
		pid := owners[s.Inode]
		if pid == 0 {
			hiddenUID[s.Port] = s.UID
		}
		listeners = append(listeners, listener{Addr: s.Addr, Port: s.Port, PID: pid})
	}

	boot, bootErr := li.bootTime()
	if bootErr != nil {
		res.Warnings = append(res.Warnings, fmt.Errorf("process ages unavailable: %w", bootErr))
	}
	now := li.now()
	for _, l := range dedupeListeners(listeners) {
		if l.PID == 0 {
			res.Entries = append(res.Entries, PortEntry{Port: l.Port, Process: "(unknown)", AgeSeconds: -1})
			res.Warnings = append(res.Warnings, fmt.Errorf("port %d: owned by %s, whose processes portfind can't see (run with sudo to see it)", l.Port, uidName(hiddenUID[l.Port])))
			continue
		}
		e, err := li.resolve(l, boot, bootErr == nil, now)
		if err != nil {
			res.Warnings = append(res.Warnings, err)
		}
		res.Entries = append(res.Entries, e)
	}
	return res, nil
}

func (li *linuxInspector) IsListening(port, pid int) (bool, error) {
	socks, err := li.listeningSockets()
	if err != nil {
		return false, err
	}
	inodes, err := li.fdSocketInodes(pid)
	if err != nil {
		return false, err
	}
	for _, s := range socks {
		if s.Port == port && inodes[s.Inode] {
			return true, nil
		}
	}
	return false, nil
}

// listeningSockets reads IPv4 and IPv6 listeners. tcp6 is absent on kernels
// built without IPv6, which is fine.
func (li *linuxInspector) listeningSockets() ([]procSocket, error) {
	var all []procSocket
	for _, name := range []string{"net/tcp", "net/tcp6"} {
		data, err := os.ReadFile(filepath.Join(li.proc, name))
		if errors.Is(err, fs.ErrNotExist) && name == "net/tcp6" {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read /proc/%s: %w", name, err)
		}
		socks, err := parseProcNetTCP(data)
		if err != nil {
			return nil, fmt.Errorf("parse /proc/%s: %w", name, err)
		}
		all = append(all, socks...)
	}
	return all, nil
}

// socketOwners maps socket inode -> PID by reading every process's fd links.
// Processes whose fds can't be read (other users' when not root) are skipped;
// their sockets have no owner in the map, and Scan reports them as unknown.
func (li *linuxInspector) socketOwners() map[uint64]int {
	owners := map[uint64]int{}
	dirs, err := os.ReadDir(li.proc)
	if err != nil {
		return owners
	}
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		inodes, err := li.fdSocketInodes(pid)
		if err != nil {
			continue
		}
		for inode := range inodes {
			if _, taken := owners[inode]; !taken { // a socket shared after fork: keep the first
				owners[inode] = pid
			}
		}
	}
	return owners
}

func (li *linuxInspector) fdSocketInodes(pid int) (map[uint64]bool, error) {
	fdDir := filepath.Join(li.proc, strconv.Itoa(pid), "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return nil, err
	}
	inodes := map[uint64]bool{}
	for _, fd := range fds {
		link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue // fd closed while we looked
		}
		if inode, ok := socketInode(link); ok {
			inodes[inode] = true
		}
	}
	return inodes, nil
}

func (li *linuxInspector) bootTime() (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(li.proc, "stat"))
	if err != nil {
		return time.Time{}, err
	}
	sec, err := parseBootTime(data)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}

// resolve fills in name, path, age and parent for a listener's process.
func (li *linuxInspector) resolve(l listener, boot time.Time, haveBoot bool, now time.Time) (PortEntry, error) {
	e := PortEntry{Port: l.Port, PID: l.PID, AgeSeconds: -1}
	st, err := li.stat(l.PID)
	if err != nil {
		e.Process = "(exited)"
		return e, fmt.Errorf("port %d pid %d: %w", l.Port, l.PID, err)
	}
	e.Process = st.Comm
	if exe, err := os.Readlink(filepath.Join(li.proc, strconv.Itoa(l.PID), "exe")); err == nil {
		e.Command = exe
		e.Process = DisplayName(exe) // untruncated, unlike comm
	}
	if haveBoot {
		started := boot.Add(time.Duration(st.StartTicks) * time.Second / clockTicks)
		e.AgeSeconds = max(int64(now.Sub(started).Seconds()), 0)
	}
	if st.PPID > 0 {
		e.ParentPID = st.PPID
		if parent, err := li.stat(st.PPID); err == nil {
			e.ParentProcess = parent.Comm
		}
	}
	return e, nil
}

func (li *linuxInspector) stat(pid int) (procStat, error) {
	data, err := os.ReadFile(filepath.Join(li.proc, strconv.Itoa(pid), "stat"))
	if err != nil {
		return procStat{}, err
	}
	return parseProcStat(data)
}

// ProcessName returns the display name of a running process: the base name
// of its executable, or its kernel comm when the executable link isn't
// readable. kill uses it to re-verify a target right before signalling.
func ProcessName(pid int) (string, error) {
	if exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe")); err == nil {
		return DisplayName(exe), nil
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	st, err := parseProcStat(data)
	if err != nil {
		return "", err
	}
	return st.Comm, nil
}

// ProcessState returns the state letter from /proc/<pid>/stat ('Z' for a
// zombie that has exited but not been reaped).
func ProcessState(pid int) (byte, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	st, err := parseProcStat(data)
	if err != nil {
		return 0, err
	}
	return st.State, nil
}
