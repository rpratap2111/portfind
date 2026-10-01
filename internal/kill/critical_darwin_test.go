package kill

import "testing"

func TestCheckAllowedDarwinCritical(t *testing.T) {
	checkAllowCases(t, []allowCase{
		{"sshd (remote lockout)", Target{PID: 812, Port: 22, Process: "sshd"}, false},
		{"mDNSResponder (DNS)", Target{PID: 600, Port: 53, Process: "mDNSResponder"}, false},
		{"WindowServer (ends the login session)", Target{PID: 400, Port: 1, Process: "WindowServer"}, false},
		{"AirPlay receiver on :5000 is fine", Target{PID: 607, Port: 5000, Process: "ControlCenter"}, true},
		{"a Linux name is fine on macOS", Target{PID: 2000, Port: 53, Process: "systemd-resolved"}, true},
	})
}
