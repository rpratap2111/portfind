package tray

import (
	"strings"
	"testing"

	"github.com/rpratap2111/portfind/internal/inspector"
)

func TestContainerMenuText(t *testing.T) {
	c := inspector.PortEntry{
		Port: 5432, PID: 0, Process: "pg", ContainerID: "bbb222", Image: "postgres:16",
		ProjectName: "shop", RiskTier: "HIGH", AgeSeconds: 160, Command: "docker container · image postgres:16",
	}

	// PID 0 would hide a process; a container is still offered.
	shown, hidden, _ := menuPorts([]inspector.PortEntry{c}, maxPortItems)
	if len(shown) != 1 || hidden != 0 {
		t.Fatalf("container not offered in the menu: shown=%d hidden=%d", len(shown), hidden)
	}
	if got := menuLabel(c); got != "pg — :5432 (shop)\tHIGH" {
		t.Errorf("menuLabel = %q", got)
	}
	if got := killLabel(c); got != "Stop container pg…" {
		t.Errorf("killLabel = %q", got)
	}
	pid, _, _ := portDetails(c)
	if !strings.HasPrefix(pid, "Docker container") || strings.Contains(pid, "PID") {
		t.Errorf("details line = %q, want no PID for a container", pid)
	}
	text := confirmText(c)
	for _, want := range []string{"Stop container pg on port 5432?", "Image:\tpostgres:16", "HIGH risk", "database container"} {
		if !strings.Contains(text, want) {
			t.Errorf("confirm text missing %q:\n%s", want, text)
		}
	}
	if got := killedTitle(c); got != "Stopped container pg on :5432" {
		t.Errorf("killedTitle = %q", got)
	}
	if got := killFailedTitle(c); got != "Couldn't stop container pg on :5432" {
		t.Errorf("killFailedTitle = %q", got)
	}
}
