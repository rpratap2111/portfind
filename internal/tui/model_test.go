package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/scan"
)

func entry(port, pid int, process, project string) inspector.PortEntry {
	return inspector.PortEntry{Port: port, PID: pid, Process: process, ProjectName: project, RiskTier: "LOW"}
}

// newTestModel builds a model whose scans return the next queued result.
func newTestModel(t *testing.T, scans ...[]inspector.PortEntry) *Model {
	t.Helper()
	i := 0
	m := Model{scan: func() (scan.Result, error) {
		res := scan.Result{Entries: scans[min(i, len(scans)-1)]}
		i++
		return res, nil
	}}
	res, err := m.scan()
	m.applyScan(res, err, time.Now())
	return &m
}

func (m *Model) send(t *testing.T, msg tea.Msg) tea.Cmd {
	t.Helper()
	next, cmd := m.Update(msg)
	*m = next.(Model)
	return cmd
}

func (m *Model) typeText(t *testing.T, s string) {
	for _, r := range s {
		m.send(t, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// refresh runs one scan the way the tick does and applies the result.
func (m *Model) refresh(t *testing.T) {
	t.Helper()
	cmd := m.send(t, tickMsg{id: m.tickID})
	if cmd == nil {
		t.Fatal("tick did not start a scan")
	}
	m.send(t, cmd())
}

func TestFilterEntries(t *testing.T) {
	ports := []inspector.PortEntry{
		entry(3000, 10, "node", "demo-web-app"),
		entry(3306, 20, "mysqld", ""),
		entry(8899, 30, "python", "git-only-repo"),
	}
	tests := []struct {
		query string
		want  []int
	}{
		{"", []int{3000, 3306, 8899}},
		{"300", []int{3000}},       // port substring
		{"33", []int{3306}},        // not a fuzzy match on 3000
		{"dwa", []int{3000}},       // fuzzy project
		{"PYTH", []int{8899}},      // case-insensitive process
		{"node 3000", []int{3000}}, // all terms must match
		{"node 8899", nil},         // ...across the same row
		{"zzz", nil},               // nothing
		{"  sql  ", []int{3306}},   // surrounding space ignored
		{"repo", []int{8899}},      // fuzzy project subsequence
		{"3", []int{3000, 3306}},   // not in 8899
	}
	for _, tt := range tests {
		got := filterEntries(ports, tt.query)
		var gotPorts []int
		for _, e := range got {
			gotPorts = append(gotPorts, e.Port)
		}
		if len(gotPorts) != len(tt.want) {
			t.Errorf("query %q: got %v, want %v", tt.query, gotPorts, tt.want)
			continue
		}
		for i := range gotPorts {
			if gotPorts[i] != tt.want[i] {
				t.Errorf("query %q: got %v, want %v", tt.query, gotPorts, tt.want)
				break
			}
		}
	}
}

func TestRefreshKeepsSelectedRowWhenRowsShift(t *testing.T) {
	m := newTestModel(t,
		[]inspector.PortEntry{entry(3000, 1, "node", ""), entry(8899, 2, "python", "")},
		// A new port appears above the selected one.
		[]inspector.PortEntry{entry(80, 9, "nginx", ""), entry(3000, 1, "node", ""), entry(8899, 2, "python", "")},
	)
	m.send(t, tea.KeyMsg{Type: tea.KeyDown})
	m.refresh(t)

	if sel, _ := m.selected(); sel.Port != 8899 {
		t.Fatalf("selected port = %d, want 8899 to stay selected", sel.Port)
	}
}

func TestRefreshClampsWhenSelectedRowDisappears(t *testing.T) {
	m := newTestModel(t,
		[]inspector.PortEntry{entry(3000, 1, "node", ""), entry(8899, 2, "python", "")},
		[]inspector.PortEntry{entry(3000, 1, "node", "")},
		[]inspector.PortEntry{},
	)
	m.send(t, tea.KeyMsg{Type: tea.KeyDown})
	m.refresh(t)
	if m.selectedIndex != 0 {
		t.Fatalf("selectedIndex = %d, want clamp to 0", m.selectedIndex)
	}
	m.refresh(t) // everything gone: must not panic
	if _, ok := m.selected(); ok {
		t.Fatal("expected no selection with empty list")
	}
	_ = m.View()
}

func TestRefreshKeepsSearchQuery(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{entry(3000, 1, "node", ""), entry(8899, 2, "python", "")})
	m.typeText(t, "pyth")
	m.refresh(t)
	if m.searchQuery != "pyth" || len(m.visible) != 1 || m.visible[0].Port != 8899 {
		t.Fatalf("query %q visible %v; want search kept and filtered", m.searchQuery, m.visible)
	}
}

func TestPrintableKeysSearchInsteadOfCommands(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{entry(3000, 1, "node", "")})
	for _, r := range "jkqr" {
		if cmd := m.send(t, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}); cmd != nil {
			t.Fatalf("key %q returned a command; printable keys must only search", r)
		}
	}
	if m.searchQuery != "jkqr" {
		t.Fatalf("searchQuery = %q, want %q", m.searchQuery, "jkqr")
	}
	m.send(t, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.searchQuery != "jkq" {
		t.Fatalf("after backspace searchQuery = %q", m.searchQuery)
	}
}

func TestStaleTickIsIgnored(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{entry(3000, 1, "node", "")})
	if cmd := m.send(t, tickMsg{id: m.tickID - 1}); cmd != nil {
		t.Fatal("stale tick started a scan")
	}
}

func TestScanErrorKeepsPreviousData(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{entry(3000, 1, "node", "")})
	m.send(t, scanDoneMsg{err: errors.New("boom"), at: time.Now()})
	if len(m.ports) != 1 || m.scanErr == nil {
		t.Fatalf("ports=%v scanErr=%v; want old data kept and error recorded", m.ports, m.scanErr)
	}
}

func TestSelectionScrollsIntoView(t *testing.T) {
	var many []inspector.PortEntry
	for i := 0; i < 50; i++ {
		many = append(many, entry(1000+i, i+1, "node", ""))
	}
	m := newTestModel(t, many)
	m.send(t, tea.WindowSizeMsg{Width: 100, Height: chromeHeight + 5})
	for i := 0; i < 12; i++ {
		m.send(t, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.selectedIndex < m.offset || m.selectedIndex >= m.offset+5 {
		t.Fatalf("selected %d not within drawn rows [%d,%d)", m.selectedIndex, m.offset, m.offset+5)
	}
}

func TestFormatAge(t *testing.T) {
	tests := map[int64]string{-1: "?", 0: "0s", 45: "45s", 60: "1m", 8100: "2h15m", 273600: "3d4h"}
	for in, want := range tests {
		if got := formatAge(in); got != want {
			t.Errorf("formatAge(%d) = %q, want %q", in, got, want)
		}
	}
}
