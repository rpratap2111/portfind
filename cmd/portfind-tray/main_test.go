//go:build windows

package main

import "testing"

func TestFlagValue(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--updated-from", "1.2.0"}, "1.2.0"},
		{[]string{"--autostart", "--updated-from=1.2.0"}, "1.2.0"},
		{[]string{"--updated-from"}, ""}, // no value
		{[]string{"--autostart"}, ""},
		{nil, ""},
	}
	for _, tt := range tests {
		if got := flagValue(tt.args, updatedFromFlag); got != tt.want {
			t.Errorf("flagValue(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}
