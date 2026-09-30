package tray

import "testing"

func TestDisplayVersion(t *testing.T) {
	for in, want := range map[string]string{"1.2.0": "v1.2.0", "v1.2.0": "v1.2.0", "dev": "dev", "": ""} {
		if got := displayVersion(in); got != want {
			t.Errorf("displayVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
