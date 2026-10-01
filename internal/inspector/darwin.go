//go:build darwin

package inspector

import (
	"fmt"
	"os"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

const procStateZombie = 5 // SZOMB in <sys/proc.h>

type darwinInspector struct {
	now  func() time.Time
	euid int
}

// New returns the PortInspector for the current OS.
func New() PortInspector {
	return &darwinInspector{now: time.Now, euid: os.Geteuid()}
}

// darwinProc is what the process table says about one process. Unlike the
// libproc calls, it is readable for every user's processes.
type darwinProc struct {
	PPID    int
	Comm    string // executable name, truncated by the kernel to 16 bytes
	Started time.Time
	Zombie  bool
}

func (di *darwinInspector) Scan() (ScanResult, error) {
	procs, err := processTable()
	if err != nil {
		return ScanResult{}, fmt.Errorf("list processes: %w", err)
	}
	pids := make([]int, 0, len(procs))
	parentOf := make(map[int]int, len(procs))
	for pid, p := range procs {
		pids = append(pids, pid)
		parentOf[pid] = p.PPID
	}
	sort.Ints(pids)

	// Walk every process's descriptors, as lsof does. Processes portfind may
	// not inspect (other users', when not root) are skipped here and picked
	// up from the system-wide socket list below.
	holders := map[uint64][]int{} // socket ID -> PIDs holding it
	portOf := map[uint64]int{}
	for _, pid := range pids {
		socks, err := listeningSockets(pid)
		if err != nil {
			continue
		}
		for _, s := range socks {
			holders[s.ID] = append(holders[s.ID], pid)
			portOf[s.ID] = s.Port
		}
	}
	var listeners []listener
	for id, held := range holders {
		listeners = append(listeners, listener{Port: portOf[id], PID: socketOwner(held, parentOf)})
	}

	var res ScanResult
	// Sockets whose owner we can't see are still listed, with PID 0, so the
	// user knows the port is taken. Root sees every process, so has none.
	hiddenUID := map[int]int{} // port -> uid of the owner we can't see
	if di.euid != 0 {
		all, err := systemListeners()
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("ports held by other users' processes can't be listed: %w", err))
		}
		for _, s := range hiddenSockets(all, holders, di.euid) {
			hiddenUID[s.Port] = s.UID
			listeners = append(listeners, listener{Port: s.Port})
		}
	}

	now := di.now()
	for _, l := range dedupeListeners(listeners) {
		if l.PID == 0 {
			res.Entries = append(res.Entries, PortEntry{Port: l.Port, Process: "(unknown)", AgeSeconds: -1})
			res.Warnings = append(res.Warnings, fmt.Errorf("port %d: owned by %s, whose processes portfind can't see (run with sudo to see it)", l.Port, uidName(hiddenUID[l.Port])))
			continue
		}
		res.Entries = append(res.Entries, resolveDarwin(l, procs, now))
	}
	return res, nil
}

func (di *darwinInspector) IsListening(port, pid int) (bool, error) {
	socks, err := listeningSockets(pid)
	if err != nil {
		return false, err
	}
	for _, s := range socks {
		if s.Port == port {
			return true, nil
		}
	}
	return false, nil
}

// resolveDarwin fills in name, path, age and parent for a listener's process.
func resolveDarwin(l listener, procs map[int]darwinProc, now time.Time) PortEntry {
	p := procs[l.PID]
	e := PortEntry{Port: l.Port, PID: l.PID, Process: p.Comm}
	e.AgeSeconds = max(int64(now.Sub(p.Started).Seconds()), 0)
	if exe, err := procPidPath(l.PID); err == nil {
		e.Command = exe
		e.Process = DisplayName(exe) // untruncated, unlike comm
	}
	if parent, ok := procs[p.PPID]; ok && p.PPID > 0 {
		e.ParentPID = p.PPID
		e.ParentProcess = parent.Comm
		if exe, err := procPidPath(p.PPID); err == nil {
			e.ParentProcess = DisplayName(exe)
		}
	}
	return e
}

// processTable reads every process from the kern.proc sysctl (what ps uses).
func processTable() (map[int]darwinProc, error) {
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	procs := make(map[int]darwinProc, len(kps))
	for i := range kps {
		procs[int(kps[i].Proc.P_pid)] = procFromKinfo(&kps[i])
	}
	return procs, nil
}

func procFromKinfo(kp *unix.KinfoProc) darwinProc {
	start := kp.Proc.P_starttime
	return darwinProc{
		PPID:    int(kp.Eproc.Ppid),
		Comm:    cString(kp.Proc.P_comm[:]),
		Started: time.Unix(start.Sec, int64(start.Usec)*1000),
		Zombie:  kp.Proc.P_stat == procStateZombie,
	}
}

// listeningSockets returns the listening TCP sockets among pid's open
// descriptors. It fails with EPERM for another user's process (when not
// root) and ESRCH for one that has exited.
func listeningSockets(pid int) ([]darwinSocket, error) {
	size, err := procPidInfo(pid, procPidListFDs, nil)
	if err != nil {
		return nil, err
	}
	// Room for descriptors opened between the two calls.
	buf := make([]byte, size+32*procFDInfoSize)
	n, err := procPidInfo(pid, procPidListFDs, buf)
	if err != nil {
		return nil, err
	}

	var socks []darwinSocket
	info := make([]byte, socketFDInfoSize)
	for _, fd := range parseFDList(buf[:min(n, len(buf))]) {
		n, err := procPidFDInfo(pid, fd, procPidFDSocketInfo, info)
		if err != nil {
			continue // fd closed while we looked
		}
		if s, ok := parseSocketFDInfo(info[:min(n, len(info))]); ok {
			socks = append(socks, s)
		}
	}
	return socks, nil
}

// systemListeners returns every listening TCP socket on the machine with its
// owner's user ID, whoever it belongs to.
func systemListeners() ([]darwinSocket, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist_n")
	if err != nil {
		return nil, fmt.Errorf("read net.inet.tcp.pcblist_n: %w", err)
	}
	socks, err := parsePCBList(buf)
	if err != nil {
		return nil, fmt.Errorf("parse net.inet.tcp.pcblist_n: %w", err)
	}
	return socks, nil
}

func processInfo(pid int) (darwinProc, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(kp.Proc.P_pid) != pid {
		// The sysctl succeeds with no data for a PID that doesn't exist.
		return darwinProc{}, unix.ESRCH
	}
	return procFromKinfo(kp), nil
}

// ProcessName returns the display name of a running process: the base name
// of its executable, or its kernel comm when the path isn't available. kill
// uses it to re-verify a target right before signalling.
func ProcessName(pid int) (string, error) {
	if exe, err := procPidPath(pid); err == nil {
		return DisplayName(exe), nil
	}
	p, err := processInfo(pid)
	if err != nil {
		return "", err
	}
	return p.Comm, nil
}

// ProcessStatus returns when pid started and whether it is a zombie that has
// exited but not been reaped. A PID together with its start time identifies
// one process even if the PID is later reused; kill relies on that, since
// macOS has no pidfd to pin a process with.
func ProcessStatus(pid int) (started time.Time, zombie bool, err error) {
	p, err := processInfo(pid)
	if err != nil {
		return time.Time{}, false, err
	}
	return p.Started, p.Zombie, nil
}

// ProcessCwd returns pid's current working directory. It fails for another
// user's process when not root.
func ProcessCwd(pid int) (string, error) {
	buf := make([]byte, vnodePathInfoSize)
	n, err := procPidInfo(pid, procPidVnodePathInfo, buf)
	if err != nil {
		return "", err
	}
	if n < vnodeCwdPathOffset+vnodePathMax {
		return "", fmt.Errorf("proc_pidinfo returned %d bytes of vnode info, want %d", n, vnodePathInfoSize)
	}
	return cString(buf[vnodeCwdPathOffset : vnodeCwdPathOffset+vnodePathMax]), nil
}

// ProcessCommandLine returns pid's arguments as one readable line. It fails
// for another user's process when not root.
func ProcessCommandLine(pid int) (string, error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", err
	}
	return parseProcArgs(buf)
}
