//go:build darwin

package inspector

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests run against the live system: they open a real listening socket
// and check that every route to it (libproc, the system-wide socket list, the
// process table) agrees, which pins the structure offsets in procinfo.go to
// what the running macOS actually returns.

func listen(t *testing.T) (port int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func TestDarwinScanFindsOwnListener(t *testing.T) {
	port, pid := listen(t), os.Getpid()
	started := time.Now()

	res, err := New().Scan()
	if err != nil {
		t.Fatal(err)
	}
	var found *PortEntry
	for i, e := range res.Entries {
		if e.Port == port {
			found = &res.Entries[i]
		}
	}
	if found == nil {
		t.Fatalf("port %d not in scan of %d entries", port, len(res.Entries))
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if found.PID != pid || found.Process != DisplayName(exe) {
		t.Errorf("entry = %+v, want PID %d process %q", *found, pid, DisplayName(exe))
	}
	if got, _ := filepath.EvalSymlinks(found.Command); got != mustEval(t, exe) {
		t.Errorf("Command = %q, want the test binary %q", found.Command, exe)
	}
	if found.AgeSeconds < 0 || found.AgeSeconds > 600 {
		t.Errorf("AgeSeconds = %d, want a few seconds", found.AgeSeconds)
	}
	if found.ParentPID != os.Getppid() || found.ParentProcess == "" {
		t.Errorf("parent = %d %q, want PID %d with a name", found.ParentPID, found.ParentProcess, os.Getppid())
	}
	if d := time.Since(started); d > 5*time.Second {
		t.Errorf("scan took %s", d)
	}
}

func TestDarwinIsListening(t *testing.T) {
	ins, pid := New(), os.Getpid()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if ok, err := ins.IsListening(port, pid); err != nil || !ok {
		t.Errorf("IsListening(own port) = %v, %v; want true", ok, err)
	}
	ln.Close()
	if ok, err := ins.IsListening(port, pid); err != nil || ok {
		t.Errorf("IsListening(closed port) = %v, %v; want false", ok, err)
	}
	if ok, err := ins.IsListening(port, 1); err == nil && os.Geteuid() != 0 {
		t.Errorf("IsListening(launchd) = %v, nil; want an error when not root", ok)
	}
}

// The system-wide list must describe our socket exactly as libproc does,
// including the kernel's socket ID that Scan uses to match the two.
func TestDarwinSystemListenersMatchLibproc(t *testing.T) {
	port := listen(t)
	own, err := listeningSockets(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	var mine darwinSocket
	for _, s := range own {
		if s.Port == port {
			mine = s
		}
	}
	if mine.ID == 0 {
		t.Fatalf("libproc doesn't show port %d among %+v", port, own)
	}

	all, err := systemListeners()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		if s.ID == mine.ID {
			if s.Port != port || s.UID != os.Geteuid() {
				t.Errorf("system list has %+v, want port %d uid %d", s, port, os.Geteuid())
			}
			return
		}
	}
	t.Errorf("socket ID %#x (port %d) not in the system list of %d listeners", mine.ID, port, len(all))
}

func TestDarwinProcessDetails(t *testing.T) {
	pid := os.Getpid()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cwd, err := ProcessCwd(pid); err != nil || cwd != mustEval(t, wd) {
		t.Errorf("ProcessCwd = %q, %v; want %q", cwd, err, mustEval(t, wd))
	}

	// The test binary's own arguments, e.g. "…/inspector.test -test.v=true".
	if cmd, err := ProcessCommandLine(pid); err != nil || !strings.Contains(cmd, filepath.Base(os.Args[0])) {
		t.Errorf("ProcessCommandLine = %q, %v; want it to contain %q", cmd, err, filepath.Base(os.Args[0]))
	}

	exe, _ := os.Executable()
	if name, err := ProcessName(pid); err != nil || name != DisplayName(exe) {
		t.Errorf("ProcessName = %q, %v; want %q", name, err, DisplayName(exe))
	}
	// Other users' processes can still be named.
	if name, err := ProcessName(1); err != nil || name != "launchd" {
		t.Errorf("ProcessName(1) = %q, %v; want launchd", name, err)
	}

	started, zombie, err := ProcessStatus(pid)
	if err != nil || zombie || time.Since(started) < 0 || time.Since(started) > 10*time.Minute {
		t.Errorf("ProcessStatus = %v, %v, %v; want a recent start", started, zombie, err)
	}
}

func TestDarwinMissingProcess(t *testing.T) {
	const pid = 99999999 // above the kernel's PID limit
	if _, _, err := ProcessStatus(pid); err == nil {
		t.Error("ProcessStatus of a missing PID should fail")
	}
	if _, err := ProcessName(pid); err == nil {
		t.Error("ProcessName of a missing PID should fail")
	}
	if _, err := ProcessCwd(pid); err == nil {
		t.Error("ProcessCwd of a missing PID should fail")
	}
	if _, err := ProcessCommandLine(pid); err == nil {
		t.Error("ProcessCommandLine of a missing PID should fail")
	}
	if _, err := New().IsListening(80, pid); err == nil {
		t.Error("IsListening of a missing PID should fail")
	}
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
