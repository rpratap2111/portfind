package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/inspector"
)

// mixedPorts is a scan in port order: containers interleaved with processes.
func mixedPorts() []inspector.PortEntry {
	return []inspector.PortEntry{
		entry(135, 1, "svchost", ""),
		containerEntry(3000, "shop-web-1", "nginx:1.27", "MEDIUM"),
		entry(3001, 2, "node", "my-app"),
		containerEntry(5432, "shop-db-1", "postgres:16", "HIGH"),
		entry(11434, 3, "ollama", ""),
	}
}

func portsOf(entries []inspector.PortEntry) []int {
	var out []int
	for _, e := range entries {
		out = append(out, e.Port)
	}
	return out
}

func headings(m *Model) []string {
	var out []string
	for _, r := range m.tableLayout() {
		if r.entry < 0 {
			out = append(out, r.heading)
		}
	}
	return out
}

func TestSectionsGroupProcessesThenContainers(t *testing.T) {
	m := newTestModel(t, mixedPorts())

	if got, want := portsOf(m.visible), []int{135, 3001, 11434, 3000, 5432}; !equal(got, want) {
		t.Fatalf("visible order = %v, want processes then containers: %v", got, want)
	}
	if got := headings(m); len(got) != 2 || got[0] != "PROCESSES · 3" || got[1] != "DOCKER CONTAINERS · 2" {
		t.Fatalf("headings = %q", got)
	}
	view := m.View()
	p, d := strings.Index(view, "PROCESSES · 3"), strings.Index(view, "DOCKER CONTAINERS · 2")
	if p < 0 || d < p {
		t.Fatal("both headings should be drawn, processes first")
	}
}

func TestNoHeadingsWithoutContainers(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{entry(135, 1, "svchost", ""), entry(3001, 2, "node", "my-app")})
	if got := headings(m); len(got) != 0 {
		t.Fatalf("headings = %q, want none when there are no containers", got)
	}
	if strings.Contains(m.View(), "PROCESSES") {
		t.Fatal("the plain list should look as it always did")
	}
}

func TestOnlyContainersGetOneHeading(t *testing.T) {
	m := newTestModel(t, mixedPorts())
	m.typeText(t, "docker")
	if got, want := portsOf(m.visible), []int{3000, 5432}; !equal(got, want) {
		t.Fatalf(`search "docker" = %v, want only the containers %v`, got, want)
	}
	if got := headings(m); len(got) != 1 || got[0] != "DOCKER CONTAINERS · 2" {
		t.Fatalf("headings = %q", got)
	}
}

func TestDockerSearchTerms(t *testing.T) {
	tests := []struct {
		query string
		want  []int
	}{
		{"docker", []int{3000, 5432}},
		{"dock", []int{3000, 5432}},
		{"postgres", []int{5432}},   // image name
		{"nginx", []int{3000}},      // image name
		{"docker 54", []int{5432}},  // combined with a port
		{"shop", []int{3000, 5432}}, // Compose project, as before
		{"node", []int{3001}},       // a process, not a container
		{"do", []int{}},             // too short to mean "docker"; no other match
	}
	for _, tt := range tests {
		got := portsOf(sectioned(filterEntries(mixedPorts(), tt.query)))
		if !equal(got, tt.want) {
			t.Errorf("query %q = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestArrowKeysCrossSections(t *testing.T) {
	m := newTestModel(t, mixedPorts())
	var seen []int
	for i := 0; i < 5; i++ {
		e, _ := m.selected()
		seen = append(seen, e.Port)
		m.send(t, key(tea.KeyDown))
	}
	if want := []int{135, 3001, 11434, 3000, 5432}; !equal(seen, want) {
		t.Fatalf("down-arrow order = %v, want %v (headings are skipped)", seen, want)
	}
}

func TestRefreshKeepsSelectionAcrossSections(t *testing.T) {
	later := append([]inspector.PortEntry{entry(80, 9, "caddy", "")}, mixedPorts()...) // a new process appears
	m := newTestModel(t, mixedPorts(), later)
	for i := 0; i < 4; i++ { // select the postgres container, the last row
		m.send(t, key(tea.KeyDown))
	}
	m.refresh(t)
	if e, _ := m.selected(); e.Port != 5432 || !e.IsContainer() {
		t.Fatalf("selected %+v after refresh, want the postgres container to stay selected", e)
	}
}

func TestScrollingKeepsSectionHeadingWithItsFirstRow(t *testing.T) {
	var many []inspector.PortEntry
	for i := 0; i < 20; i++ {
		many = append(many, entry(1000+i, i+1, "svc", ""))
	}
	many = append(many, containerEntry(8080, "web", "nginx", "MEDIUM"), containerEntry(8081, "api", "node:22", "MEDIUM"))
	m := newTestModel(t, many)
	m.send(t, tea.WindowSizeMsg{Width: 120, Height: chromeHeight + 6}) // 6 table rows

	for i := 0; i < 21; i++ { // down to the last container
		m.send(t, key(tea.KeyDown))
	}
	if e, _ := m.selected(); e.Port != 8081 {
		t.Fatalf("selected :%d, want :8081", e.Port)
	}
	if !strings.Contains(m.View(), "8081") {
		t.Fatal("the selected row scrolled out of view")
	}

	m.send(t, key(tea.KeyUp)) // first container: its heading must come into view with it
	view := m.View()
	if !strings.Contains(view, "DOCKER CONTAINERS · 2") || !strings.Contains(view, "8080") {
		t.Fatal("moving to a section's first row should show its heading")
	}
	if first, last, scrolls := m.shownEntries(); !scrolls || first < 1 || last != 22 {
		t.Fatalf("shownEntries = %d–%d scrolls=%v, want it to end at entry 22", first, last, scrolls)
	}
}

func equal(a, b []int) bool {
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
