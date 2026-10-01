//go:build darwin

package inspector

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// libproc is how macOS exposes another process's open files, executable path
// and working directory (it is what lsof and Activity Monitor use). It is not
// wrapped by x/sys/unix, so the three calls portfind needs are bound here the
// way x/sys binds libSystem: a dynamic import plus an assembly trampoline
// (libproc_darwin.s). No cgo is involved, so portfind still cross-compiles.

//go:cgo_import_dynamic libc_proc_pidinfo proc_pidinfo "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_proc_pidfdinfo proc_pidfdinfo "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic libc_proc_pidpath proc_pidpath "/usr/lib/libSystem.B.dylib"

var (
	libc_proc_pidinfo_trampoline_addr   uintptr
	libc_proc_pidfdinfo_trampoline_addr uintptr
	libc_proc_pidpath_trampoline_addr   uintptr
)

// Implemented in the runtime; this is the entry point x/sys/unix uses for
// every libSystem call.
//
//go:linkname syscall_syscall6 syscall.syscall6
func syscall_syscall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)

// procPidInfo calls proc_pidinfo(pid, flavor, 0, buf, len(buf)) and returns
// the number of bytes written. A nil buf asks how large a buffer is needed.
func procPidInfo(pid, flavor int, buf []byte) (int, error) {
	p := bufPtr(buf)
	r, _, _ := syscall_syscall6(libc_proc_pidinfo_trampoline_addr,
		uintptr(pid), uintptr(flavor), 0, uintptr(p), uintptr(len(buf)), 0)
	return procResult(pid, r)
}

// procPidFDInfo calls proc_pidfdinfo(pid, fd, flavor, buf, len(buf)).
func procPidFDInfo(pid int, fd int32, flavor int, buf []byte) (int, error) {
	p := bufPtr(buf)
	r, _, _ := syscall_syscall6(libc_proc_pidfdinfo_trampoline_addr,
		uintptr(pid), uintptr(fd), uintptr(flavor), uintptr(p), uintptr(len(buf)), 0)
	return procResult(pid, r)
}

// procPidPath returns the path of pid's executable. Unlike the calls above,
// it works for other users' processes too.
func procPidPath(pid int) (string, error) {
	buf := make([]byte, procPidPathMaxSize)
	p := bufPtr(buf)
	r, _, _ := syscall_syscall6(libc_proc_pidpath_trampoline_addr,
		uintptr(pid), uintptr(p), uintptr(len(buf)), 0, 0, 0)
	n, err := procResult(pid, r)
	if err != nil {
		return "", err
	}
	return string(buf[:min(n, len(buf))]), nil
}

// procResult turns a libproc return value into (bytes, error). libproc
// returns 0 on failure and leaves the reason in errno, which can't be read
// reliably from Go (the goroutine may have moved to another thread). Its only
// failures for the calls above are a process that is gone and one portfind
// may not inspect, and kill(pid, 0) tells those apart.
func procResult(pid int, r uintptr) (int, error) {
	if n := int(int32(r)); n > 0 {
		return n, nil
	}
	if err := unix.Kill(pid, 0); err == unix.ESRCH {
		return 0, unix.ESRCH
	}
	return 0, unix.EPERM
}

func bufPtr(buf []byte) unsafe.Pointer {
	if len(buf) == 0 {
		return nil
	}
	return unsafe.Pointer(&buf[0])
}
