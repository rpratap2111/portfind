package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/scan"
)

func tempStore(t *testing.T) *history.Store {
	t.Helper()
	s, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newHistoryModel is newTestModel with a history store attached before the
// first scan, as New does.
func newHistoryModel(t *testing.T, store historyStore, scans ...[]inspector.PortEntry) *Model {
	t.Helper()
	i := 0
	m := Model{hist: store, scan: func() (scan.Result, error) {
		res := scan.Result{Entries: scans[min(i, len(scans)-1)]}
		i++
		return res, nil
	}}
	res, err := m.scan()
	m.applyScan(res, err, time.Now())
	return &m
}

// killSelected kills the selected LOW-risk row through the dialog and runs
// the rescan that follows.
func (m *Model) killSelected(t *testing.T) {
	t.Helper()
	withFakeKill(m, nil)
	m.send(t, key(tea.KeyEnter))
	rescan := m.runCmd(t, m.send(t, runes("y")))
	m.runCmd(t, rescan)
}

func TestDeparturesAreLoggedWithHowTheyLeft(t *testing.T) {
	store := tempStore(t)
	node := tiered(3000, 10, "node", "LOW")
	node.ProjectName = "demo-web-app"
	py := tiered(8899, 20, "python", "LOW")
	m := newHistoryModel(t, store,
		[]inspector.PortEntry{node, py},
		[]inspector.PortEntry{py}, // node killed via portfind
		[]inspector.PortEntry{},   // python exits on its own
	)
	m.killSelected(t)
	m.refresh(t)

	events, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want 2", events)
	}
	byPort := map[int]history.Event{}
	for _, e := range events {
		byPort[e.Port] = e
	}
	if e := byPort[3000]; !e.KilledViaPortfind || e.Process != "node" || e.Project != "demo-web-app" || e.PID != 10 {
		t.Fatalf("node event = %+v, want killed via portfind with project", e)
	}
	if e := byPort[8899]; e.KilledViaPortfind {
		t.Fatalf("python event = %+v, want exited on its own", e)
	}
	if len(m.killedPIDs) != 0 {
		t.Fatalf("killedPIDs = %v, want cleared once the PID is gone", m.killedPIDs)
	}
}

func TestKilledProcessWithSeveralPortsLogsEachAsKilled(t *testing.T) {
	store := tempStore(t)
	m := newHistoryModel(t, store,
		[]inspector.PortEntry{tiered(3306, 7, "mysqld-fake", "LOW"), tiered(33060, 7, "mysqld-fake", "LOW")},
		[]inspector.PortEntry{},
	)
	m.killSelected(t)
	events, _ := store.Recent(10)
	if len(events) != 2 || !events[0].KilledViaPortfind || !events[1].KilledViaPortfind {
		t.Fatalf("events = %+v, want both ports logged as killed", events)
	}
}

func TestFightHintAfterThirdKill(t *testing.T) {
	store := tempStore(t)
	up := func(pid int) []inspector.PortEntry { return []inspector.PortEntry{tiered(3000, pid, "node", "LOW")} }
	// Each kill is followed by a respawn with a new PID, like nodemon.
	m := newHistoryModel(t, store, up(1), nil, up(2), nil, up(3), nil)
	for i := 1; i <= 3; i++ {
		if len(m.fights) != 0 {
			t.Fatalf("hint shown after only %d kills", i-1)
		}
		m.killSelected(t)
		if i < 3 {
			m.refresh(t) // respawn appears
		}
	}
	if len(m.fights) != 1 || m.fights[0].Port != 3000 || m.fights[0].Kills != 3 {
		t.Fatalf("fights = %+v, want :3000 x3", m.fights)
	}
	if v := m.View(); !strings.Contains(v, ":3000 killed 3× in 15m (node)") {
		t.Fatal("fight hint not rendered")
	}
}

func TestFightHintOnSessionStart(t *testing.T) {
	store := tempStore(t)
	for pid := 1; pid <= 3; pid++ {
		err := store.Record(history.Event{At: time.Now().Add(-time.Duration(pid) * time.Minute),
			Port: 5173, PID: pid, Process: "node", Project: "vite-app", KilledViaPortfind: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := newHistoryModel(t, store, []inspector.PortEntry{})
	if !strings.Contains(m.View(), ":5173 killed 3× in 15m (node · vite-app)") {
		t.Fatal("fight from a previous session not hinted at startup")
	}
}

func TestHistoryViewKeys(t *testing.T) {
	store := tempStore(t)
	m := newHistoryModel(t, store, []inspector.PortEntry{tiered(3000, 1, "node", "LOW")}, nil)
	m.killSelected(t)

	m.send(t, key(tea.KeyTab))
	if !m.showHistory || len(m.historyRows) != 1 {
		t.Fatalf("showHistory=%v rows=%d, want history view with the kill", m.showHistory, len(m.historyRows))
	}
	if !strings.Contains(m.View(), "killed by portfind") {
		t.Fatal("history view does not show the kill")
	}
	m.send(t, key(tea.KeyEnter))
	if m.confirmingKill {
		t.Fatal("Enter in the history view must not open a kill dialog")
	}
	if cmd := m.send(t, key(tea.KeyEsc)); cmd != nil || m.showHistory {
		t.Fatal("Esc in the history view should return to ports, not quit")
	}
	if cmd := m.send(t, key(tea.KeyEsc)); cmd == nil {
		t.Fatal("Esc on the port list should quit")
	}
}

type failingStore struct{}

func (failingStore) Record(history.Event) error                         { return errors.New("disk I/O error") }
func (failingStore) Recent(int) ([]history.Event, error)                { return nil, nil }
func (failingStore) PortFights(time.Time, int) ([]history.Fight, error) { return nil, nil }

func TestHistoryWriteErrorIsSurfaced(t *testing.T) {
	m := newHistoryModel(t, failingStore{},
		[]inspector.PortEntry{tiered(3000, 1, "node", "LOW")},
		[]inspector.PortEntry{},
	)
	m.refresh(t)
	if m.histErr == nil || !strings.Contains(m.View(), "history error (ctrl+w)") {
		t.Fatal("history write error not shown in the status line")
	}
	m.send(t, tea.KeyMsg{Type: tea.KeyCtrlW})
	if !strings.Contains(m.View(), "history: disk I/O error") {
		t.Fatal("history write error not listed in the warnings view")
	}
}
