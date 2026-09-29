package kill

import "testing"

type allowCase struct {
	name    string
	target  Target
	allowed bool
}

func checkAllowCases(t *testing.T, cases []allowCase) {
	t.Helper()
	for _, tt := range cases {
		err := CheckAllowed(tt.target)
		if (err == nil) != tt.allowed {
			t.Errorf("%s: CheckAllowed(%+v) = %v, want allowed=%v", tt.name, tt.target, err, tt.allowed)
		}
	}
}

func TestCheckAllowed(t *testing.T) {
	checkAllowCases(t, []allowCase{
		{"dev server", Target{PID: 1234, Port: 3000, Process: "node"}, true},
		{"database is allowed after confirmation", Target{PID: 8804, Port: 5432, Process: "postgres"}, true},
		{"PID 0 (unknown owner, or Windows idle)", Target{PID: 0, Port: 1, Process: "(unknown)"}, false},
		{"PID 1 (init / systemd)", Target{PID: 1, Port: 5355, Process: "whatever"}, false},
		{"PID 4 (Windows kernel)", Target{PID: 4, Port: 445, Process: "System"}, false},
		{"unknown name", Target{PID: 999, Port: 80, Process: ""}, false},
	})
}
