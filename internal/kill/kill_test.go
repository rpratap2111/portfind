package kill

import "testing"

func TestCheckAllowed(t *testing.T) {
	tests := []struct {
		name    string
		target  Target
		allowed bool
	}{
		{"dev server", Target{PID: 1234, Port: 3000, Process: "node"}, true},
		{"database is allowed after confirmation", Target{PID: 8804, Port: 5432, Process: "postgres"}, true},
		{"idle pseudo-process", Target{PID: 0, Port: 1, Process: "System Idle Process"}, false},
		{"kernel", Target{PID: 4, Port: 445, Process: "System"}, false},
		{"lsass", Target{PID: 1676, Port: 49664, Process: "lsass"}, false},
		{"case-insensitive", Target{PID: 1504, Port: 49665, Process: "WinInit"}, false},
		{"svchost", Target{PID: 1984, Port: 135, Process: "svchost"}, false},
		{"unknown name", Target{PID: 999, Port: 80, Process: ""}, false},
	}
	for _, tt := range tests {
		err := CheckAllowed(tt.target)
		if (err == nil) != tt.allowed {
			t.Errorf("%s: CheckAllowed(%+v) = %v, want allowed=%v", tt.name, tt.target, err, tt.allowed)
		}
	}
}
