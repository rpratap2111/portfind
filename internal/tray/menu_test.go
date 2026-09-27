package tray

import (
	"strings"
	"testing"
	"time"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
)

func pe(port, pid int, process, tier, command string) inspector.PortEntry {
	return inspector.PortEntry{Port: port, PID: pid, Process: process, RiskTier: tier, Command: command, AgeSeconds: 90}
}

func TestMenuPorts(t *testing.T) {
	withProject := pe(9300, 50, "mystery-daemon", "MEDIUM", "mystery-daemon 9300")
	withProject.ProjectName = "fixtures"
	all := []inspector.PortEntry{ // scan order: by port
		pe(135, 1984, "svchost", "MEDIUM", ""),          // critical and inaccessible
		pe(445, 4, "System", "MEDIUM", ""),              // kernel
		pe(3000, 10, "node", "LOW", "node server.js"),   // dev server
		pe(3306, 7840, "mysqld", "HIGH", ""),            // inaccessible (service)
		pe(5432, 20, "postgres", "HIGH", "postgres -D"), // database, accessible
		pe(8080, 30, "caddy", "MEDIUM", "caddy run"),    // unrecognized, no project
		pe(8899, 40, "python", "LOW", "python -m http"), // dev server
		withProject, // unrecognized, but from a project
		pe(49738, 60, "language_server", "MEDIUM", "language_server"), // IDE helper noise
	}

	tests := []struct {
		name         string
		limit        int
		wantPorts    []int
		wantHidden   int
		wantOverflow int
	}{
		{"LOW, then project, then HIGH, then noise; port order within each", 12, []int{3000, 8899, 9300, 5432, 8080, 49738}, 3, 0},
		{"noise can't push databases or projects out", 4, []int{3000, 8899, 9300, 5432}, 3, 2},
		{"limit keeps dev servers first", 2, []int{3000, 8899}, 3, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown, hidden, overflow := menuPorts(all, tt.limit)
			var ports []int
			for _, e := range shown {
				ports = append(ports, e.Port)
			}
			if !equalInts(ports, tt.wantPorts) || hidden != tt.wantHidden || overflow != tt.wantOverflow {
				t.Fatalf("got ports=%v hidden=%d overflow=%d, want %v %d %d",
					ports, hidden, overflow, tt.wantPorts, tt.wantHidden, tt.wantOverflow)
			}
		})
	}
}

func TestMenuPortsEmpty(t *testing.T) {
	shown, hidden, overflow := menuPorts(nil, maxPortItems)
	if len(shown) != 0 || hidden != 0 || overflow != 0 {
		t.Fatalf("got %v %d %d for no entries", shown, hidden, overflow)
	}
}

func TestMenuLabel(t *testing.T) {
	tests := []struct {
		entry inspector.PortEntry
		want  string
	}{
		{inspector.PortEntry{Process: "node", Port: 3000, ProjectName: "my-app", RiskTier: "LOW"}, "node — :3000 (my-app)\tLOW"},
		{inspector.PortEntry{Process: "caddy", Port: 8080, RiskTier: "MEDIUM"}, "caddy — :8080\tMEDIUM"},
		// & would otherwise become a keyboard mnemonic; control chars are flattened.
		{inspector.PortEntry{Process: "node", Port: 1, ProjectName: "R&D\ttools", RiskTier: "LOW"}, "node — :1 (R&&D tools)\tLOW"},
	}
	for _, tt := range tests {
		if got := menuLabel(tt.entry); got != tt.want {
			t.Errorf("menuLabel(%+v) = %q, want %q", tt.entry, got, tt.want)
		}
	}
}

func TestPortDetailsAndKillLabel(t *testing.T) {
	low := pe(3000, 42, "node", "LOW", "node server.js")
	low.ProjectName = "R&D"
	pid, project, command := portDetails(low)
	if pid != "PID 42 · running 1m · LOW risk" || project != "Project: R&&D" || command != "node server.js" {
		t.Errorf("portDetails(low) = %q, %q, %q", pid, project, command)
	}
	if got := killLabel(low); got != "Kill node" {
		t.Errorf("killLabel(LOW) = %q, want no ellipsis (kills without a dialog)", got)
	}

	high := pe(5432, 7, "postgres", "HIGH", strings.Repeat("y", 100))
	_, project, command = portDetails(high)
	if project != "No project detected" || len([]rune(command)) != 70 {
		t.Errorf("portDetails(high) project=%q command len=%d, want placeholder and 70", project, len([]rune(command)))
	}
	if got := killLabel(high); got != "Kill postgres…" {
		t.Errorf("killLabel(HIGH) = %q, want an ellipsis (a dialog follows)", got)
	}
}

func TestConfirmText(t *testing.T) {
	high := pe(5432, 8804, "postgres", "HIGH", `"C:\pg\postgres.exe" -D data`)
	got := confirmText(high)
	for _, want := range []string{"Kill postgres on port 5432?", "none detected", "PID:\t8804 (running for 1m)", `postgres.exe" -D data`, "HIGH risk", "database"} {
		if !strings.Contains(got, want) {
			t.Errorf("HIGH confirm text missing %q:\n%s", want, got)
		}
	}

	medium := pe(9300, 5, "mystery-daemon", "MEDIUM", strings.Repeat("x", 500))
	medium.ProjectName = "fixtures"
	got = confirmText(medium)
	if !strings.Contains(got, "Project:\tfixtures") || !strings.Contains(got, "MEDIUM risk") || strings.Contains(got, "database") {
		t.Errorf("MEDIUM confirm text wrong:\n%s", got)
	}
	if strings.Contains(got, strings.Repeat("x", 201)) {
		t.Error("long command lines should be truncated in the dialog")
	}
}

func TestFightLabel(t *testing.T) {
	if got := fightLabel(nil); got != "" {
		t.Errorf("fightLabel(nil) = %q, want empty", got)
	}
	got := fightLabel([]history.Fight{{Port: 3000, Kills: 3, Last: time.Now()}, {Port: 8080, Kills: 4}})
	if !strings.Contains(got, ":3000 (3×), :8080 (4×)") || !strings.Contains(got, "15m") {
		t.Errorf("fightLabel = %q", got)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
