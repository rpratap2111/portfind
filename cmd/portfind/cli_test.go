package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    mode
		wantErr string
	}{
		{nil, modeTUI, ""},
		{[]string{"--json"}, modeJSON, ""},
		{[]string{"-json"}, modeJSON, ""},
		{[]string{"--update"}, modeUpdate, ""},
		{[]string{"--version"}, modeVersion, ""},
		{[]string{"-v"}, modeVersion, ""},
		{[]string{"--help"}, modeHelp, ""},
		{[]string{"-h"}, modeHelp, ""},
		{[]string{"--json", "--update"}, 0, "only one of"},
		{[]string{"--frobnicate"}, 0, "flag provided but not defined"},
		{[]string{"kill"}, 0, `unexpected argument "kill"`},
	}
	for _, tt := range tests {
		got, err := parseArgs(tt.args)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseArgs(%v) err = %v, want %q", tt.args, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parseArgs(%v) = %v, %v; want %v", tt.args, got, err, tt.want)
		}
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.2.3")
	for _, want := range []string{"portfind v1.2.3", "--json", "--update", "--version", "--help", "Enter", "Tab", "Esc"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("help text missing %q", want)
		}
	}
}

func TestDisplayVersion(t *testing.T) {
	old := version
	defer func() { version = old }()
	for _, tt := range []struct{ stamped, want string }{
		{"1.1.1", "v1.1.1"},
		{"v1.1.1", "v1.1.1"}, // never "vv"
		{"1.0.0-SNAPSHOT-ed0775a", "v1.0.0-SNAPSHOT-ed0775a"},
	} {
		version = tt.stamped
		if got := displayVersion(); got != tt.want {
			t.Errorf("stamped %q: displayVersion() = %q, want %q", tt.stamped, got, tt.want)
		}
	}
}
