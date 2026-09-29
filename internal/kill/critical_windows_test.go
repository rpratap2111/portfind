package kill

import "testing"

func TestCheckAllowedWindowsCritical(t *testing.T) {
	checkAllowCases(t, []allowCase{
		{"lsass", Target{PID: 1676, Port: 49664, Process: "lsass"}, false},
		{"case-insensitive", Target{PID: 1504, Port: 49665, Process: "WinInit"}, false},
		{"svchost", Target{PID: 1984, Port: 135, Process: "svchost"}, false},
		{"a Linux name is fine on Windows", Target{PID: 2000, Port: 22, Process: "sshd"}, true},
	})
}
