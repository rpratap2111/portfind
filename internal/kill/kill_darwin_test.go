//go:build darwin

package kill

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// listenerEnv makes the test binary act as a server instead of running
// tests: it listens on a port, prints the port, and waits to be killed.
// "stubborn" also ignores SIGTERM, as a hung dev server would.
const listenerEnv = "PORTFIND_TEST_LISTENER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(listenerEnv); mode != "" {
		if mode == "stubborn" {
			signal.Ignore(syscall.SIGTERM)
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(ln.Addr().(*net.TCPAddr).Port)
		for {
			if conn, err := ln.Accept(); err == nil {
				conn.Close()
			}
		}
	}
	os.Exit(m.Run())
}

// startListener runs a child process that listens on a port of its choosing.
func startListener(t *testing.T, mode string) (cmd *exec.Cmd, target Target) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(exe)
	cmd.Env = append(os.Environ(), listenerEnv+"="+mode)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("read the listener's port: %v", err)
	}
	port, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}
	return cmd, Target{PID: cmd.Process.Pid, Port: port, Process: inspector.DisplayName(exe)}
}

// alive reports whether the child is still running (not even as a zombie).
func alive(pid int) bool {
	_, zombie, err := inspector.ProcessStatus(pid)
	return err == nil && !zombie
}

func TestTerminateDarwin(t *testing.T) {
	ins := inspector.New()
	_, target := startListener(t, "polite")

	start := time.Now()
	if err := Terminate(ins, target); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if alive(target.PID) {
		t.Error("the process is still running after Terminate returned nil")
	}
	// SIGTERM is enough for a process that doesn't ignore it. The child is
	// an unreaped zombie here, which must count as exited.
	if d := time.Since(start); d > gracePeriod {
		t.Errorf("Terminate took %s; it should not have waited out the grace period", d)
	}

	err := Terminate(ins, target)
	if err == nil {
		t.Error("Terminate of an exited process should fail")
	}
}

func TestTerminateDarwinEscalatesToSIGKILL(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the SIGTERM grace period")
	}
	cmd, target := startListener(t, "stubborn")

	start := time.Now()
	if err := Terminate(inspector.New(), target); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if d := time.Since(start); d < gracePeriod {
		t.Errorf("Terminate returned after %s, before the %s grace period", d, gracePeriod)
	}
	err := cmd.Wait()
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || ws.Signal() != syscall.SIGKILL {
		t.Errorf("child ended with %v, want killed by SIGKILL", err)
	}
}

// A target that no longer matches what the user saw must be left alone.
func TestTerminateDarwinRefusesMismatch(t *testing.T) {
	ins := inspector.New()
	_, target := startListener(t, "polite")

	renamed := target
	renamed.Process = "node"
	if err := Terminate(ins, renamed); err == nil || !strings.Contains(err.Error(), "PID was reused") {
		t.Errorf("wrong process name: err = %v, want a PID-reuse refusal", err)
	}

	otherPort := target
	ln, err := net.Listen("tcp", "127.0.0.1:0") // a port the child isn't on
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	otherPort.Port = ln.Addr().(*net.TCPAddr).Port
	if err := Terminate(ins, otherPort); err == nil || !strings.Contains(err.Error(), "no longer listening") {
		t.Errorf("wrong port: err = %v, want a no-longer-listening refusal", err)
	}

	if !alive(target.PID) {
		t.Error("a refused Terminate killed the process")
	}
}

// gone must treat a reused PID (same number, later start time) as gone.
func TestGoneDarwinDetectsPIDReuse(t *testing.T) {
	pid := os.Getpid()
	started, _, err := inspector.ProcessStatus(pid)
	if err != nil {
		t.Fatal(err)
	}
	if gone(pid, started) {
		t.Error("gone(self) = true")
	}
	if !gone(pid, started.Add(-time.Hour)) {
		t.Error("gone = false for a PID whose process started at a different time")
	}
}
