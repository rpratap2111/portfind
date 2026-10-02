package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/kill"
)

func containerEntry(port int, name, image, tier string) inspector.PortEntry {
	return inspector.PortEntry{
		Port: port, PID: 700, Process: name, ContainerID: "id-" + name, Image: image,
		ProjectName: "shop", RiskTier: tier, AgeSeconds: 160, Command: "docker container · image " + image,
	}
}

func TestContainerRowAndDialog(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{containerEntry(8080, "shop-web-1", "nginx:1.27", "MEDIUM")})
	calls := withFakeKill(m, nil)

	view := m.View()
	if !strings.Contains(view, "docker") || strings.Contains(view, " 700 ") {
		t.Error("the PID column should say docker, not Docker's own PID")
	}

	m.send(t, key(tea.KeyEnter))
	view = m.View()
	for _, want := range []string{"Stop container shop-web-1 on :8080?", "Docker container", `type "shop-web-1" to confirm`} {
		if !strings.Contains(view, want) {
			t.Errorf("dialog missing %q", want)
		}
	}

	m.send(t, runes("y")) // never a plain y/N for a container
	if len(*calls) != 0 {
		t.Fatal("a container must not be stopped with a plain y")
	}
	m.send(t, key(tea.KeyBackspace))
	m.send(t, runes("shop-web-1"))
	m.runCmd(t, m.send(t, key(tea.KeyEnter)))

	want := kill.Target{PID: 700, Port: 8080, Process: "shop-web-1", ContainerID: "id-shop-web-1"}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("kill calls = %+v, want %+v", *calls, want)
	}
	if !strings.Contains(m.status, "Stopped container shop-web-1 on :8080") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestStoppedContainerHistoryDoesNotTaintOtherPorts(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	web := containerEntry(8080, "web", "nginx", "MEDIUM")
	api := containerEntry(9090, "api", "node:22", "MEDIUM") // same Docker PID, different container
	m := newHistoryModel(t, store,
		[]inspector.PortEntry{web, api},
		[]inspector.PortEntry{api}, // web stopped via portfind
		[]inspector.PortEntry{},    // api stopped some other way (docker compose down)
	)
	withFakeKill(m, nil)
	m.send(t, key(tea.KeyEnter))
	m.send(t, runes("web"))
	rescan := m.runCmd(t, m.send(t, key(tea.KeyEnter)))
	m.runCmd(t, rescan)
	m.refresh(t)

	events, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	killed := map[int]bool{}
	for _, e := range events {
		killed[e.Port] = e.KilledViaPortfind
	}
	if len(events) != 2 || !killed[8080] || killed[9090] {
		t.Fatalf("events = %+v; want :8080 killed via portfind and :9090 not", events)
	}
}
