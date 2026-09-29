package kill

import "testing"

func TestCheckAllowedLinuxCritical(t *testing.T) {
	checkAllowCases(t, []allowCase{
		{"sshd (remote lockout)", Target{PID: 812, Port: 22, Process: "sshd"}, false},
		{"systemd-resolved (DNS)", Target{PID: 600, Port: 53, Process: "systemd-resolved"}, false},
		{"systemd user instance", Target{PID: 1500, Port: 1, Process: "systemd"}, false},
		{"a Windows name is fine on Linux", Target{PID: 2000, Port: 135, Process: "svchost"}, true},
	})
}
