// Package tui is the interactive terminal front-end, built on Bubble Tea's
// Elm architecture: model.go holds state, update.go handles messages and
// view.go renders.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"portfind/internal/history"
	"portfind/internal/inspector"
	"portfind/internal/kill"
	"portfind/internal/risk"
	"portfind/internal/scan"
)

const refreshInterval = 2 * time.Second

// scanFunc produces a fresh annotated scan and killFunc terminates a process.
// Both are injected so tests can use fakes.
type (
	scanFunc func() (scan.Result, error)
	killFunc func(kill.Target) error
)

// historyStore is the subset of *history.Store the TUI uses.
type historyStore interface {
	Record(history.Event) error
	Recent(limit int) ([]history.Event, error)
	PortFights(since time.Time, threshold int) ([]history.Fight, error)
}

// historyLimit caps how many events the history view loads.
const historyLimit = 500

// statusKind picks the colour of the status line.
type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusError
)

// Model is the TUI state.
type Model struct {
	scan scanFunc
	kill killFunc

	ports   []inspector.PortEntry // latest successful scan, sorted by port
	visible []inspector.PortEntry // subset of ports matching searchQuery

	searchQuery   string
	selectedIndex int // index into visible
	offset        int // index of the first row drawn, for scrolling

	// Kill confirmation. pendingKill is a copy taken when the dialog opened,
	// so background refreshes can't change what the user is confirming.
	confirmingKill    bool
	pendingKill       *inspector.PortEntry
	typedConfirmation string // MEDIUM/HIGH: what the user has typed so far
	confirmErr        string // e.g. typed name doesn't match
	killing           bool   // a kill command is in flight

	// History. hist is nil when the database could not be opened; histErr
	// then (or after any failed read/write) says why and is shown to the user.
	hist        historyStore
	histErr     error
	killedPIDs  map[int]bool    // killed by portfind, not yet seen gone by a scan
	fights      []history.Fight // port fights in the current window
	historyRows []history.Event // loaded while the history view is open
	showHistory bool

	warnings     []error // from the latest successful scan
	scanErr      error   // last scan failure; stale data stays on screen
	lastScan     time.Time
	scanning     bool
	tickID       int // only the tick carrying the current ID triggers a scan
	status       string
	statusKind   statusKind
	showWarnings bool

	width, height int
}

// New builds the model and runs the first scan synchronously, so the very
// first frame already shows real ports instead of a loading placeholder.
// store may be nil if the history database failed to open, in which case
// storeErr explains why; the TUI then runs without history.
func New(ins inspector.PortInspector, store *history.Store, storeErr error) Model {
	m := Model{
		scan:    func() (scan.Result, error) { return scan.Run(ins) },
		kill:    func(t kill.Target) error { return kill.Terminate(ins, t) },
		histErr: storeErr,
	}
	if store != nil { // avoid a typed-nil interface
		m.hist = store
	}
	res, err := m.scan()
	m.applyScan(res, err, time.Now()) // also runs the session-start fight check
	return m
}

// Init starts the auto-refresh loop.
func (m Model) Init() tea.Cmd {
	return tickCmd(m.tickID)
}

type tickMsg struct{ id int }

type killDoneMsg struct {
	target kill.Target
	err    error
}

type scanDoneMsg struct {
	res scan.Result
	err error
	at  time.Time
}

func tickCmd(id int) tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{id: id} })
}

func (m Model) scanCmd() tea.Cmd {
	run := m.scan
	return func() tea.Msg {
		res, err := run()
		return scanDoneMsg{res: res, err: err, at: time.Now()}
	}
}

// rowKey identifies a row across refreshes.
type rowKey struct{ port, pid int }

func keyOf(e inspector.PortEntry) rowKey { return rowKey{e.Port, e.PID} }

// applyScan swaps in new scan data while keeping the user's place: the same
// (port, PID) stays selected if it still exists, otherwise the index is
// clamped. A failed scan keeps the previous data and records the error.
func (m *Model) applyScan(res scan.Result, err error, at time.Time) {
	if err != nil {
		m.scanErr = err
		return
	}
	m.scanErr = nil
	if !m.lastScan.IsZero() {
		m.recordDepartures(res.Entries, at)
	}
	m.ports = res.Entries
	m.warnings = res.Warnings
	m.lastScan = at

	prev, hadSelection := m.selected()
	m.visible = filterEntries(m.ports, m.searchQuery)
	if hadSelection {
		for i, e := range m.visible {
			if keyOf(e) == keyOf(prev) {
				m.selectedIndex = i
				break
			}
		}
	}
	m.clampSelection()
	m.refreshHistory(at)
}

// departures returns the entries in prev whose (port, PID) is absent from next.
func departures(prev, next []inspector.PortEntry) []inspector.PortEntry {
	current := make(map[rowKey]bool, len(next))
	for _, e := range next {
		current[keyOf(e)] = true
	}
	var gone []inspector.PortEntry
	for _, e := range prev {
		if !current[keyOf(e)] {
			gone = append(gone, e)
		}
	}
	return gone
}

// recordDepartures logs processes that were listening in the previous scan
// but not in next. Departures of a PID portfind killed are logged as kills
// (for every port it held); anything else went away on its own or was killed
// elsewhere.
func (m *Model) recordDepartures(next []inspector.PortEntry, at time.Time) {
	if m.hist == nil {
		return
	}
	for _, e := range departures(m.ports, next) {
		err := m.hist.Record(history.Event{
			At: at, Port: e.Port, PID: e.PID, Process: e.Process, Project: e.ProjectName,
			KilledViaPortfind: m.killedPIDs[e.PID],
		})
		if err != nil {
			m.histErr = err
		}
	}
	// Forget killed PIDs once they hold no ports, before Windows can reuse
	// the number for an unrelated process.
	alive := make(map[int]bool, len(next))
	for _, e := range next {
		alive[e.PID] = true
	}
	for pid := range m.killedPIDs {
		if !alive[pid] {
			delete(m.killedPIDs, pid)
		}
	}
}

// refreshHistory re-runs port-fight detection (at session start and after
// every scan, so the hint appears after a kill and fades once the window
// passes) and reloads the history view if it is open.
func (m *Model) refreshHistory(now time.Time) {
	if m.hist == nil {
		return
	}
	fights, err := m.hist.PortFights(now.Add(-history.FightWindow), history.FightThreshold)
	if err != nil {
		m.histErr = err
	} else {
		m.fights = fights
	}
	if m.showHistory {
		m.loadHistoryRows()
	}
}

func (m *Model) loadHistoryRows() {
	if m.hist == nil {
		m.historyRows = nil
		return
	}
	rows, err := m.hist.Recent(historyLimit)
	if err != nil {
		m.histErr = err
		return
	}
	m.historyRows = rows
}

func (m *Model) markKilled(pid int) {
	if m.killedPIDs == nil {
		m.killedPIDs = make(map[int]bool)
	}
	m.killedPIDs[pid] = true
}

// setQuery re-filters and jumps to the best (first) match, as fzf does.
func (m *Model) setQuery(q string) {
	m.searchQuery = q
	m.visible = filterEntries(m.ports, q)
	m.selectedIndex = 0
	m.offset = 0
	m.clampSelection()
}

func (m Model) selected() (inspector.PortEntry, bool) {
	if m.selectedIndex < 0 || m.selectedIndex >= len(m.visible) {
		return inspector.PortEntry{}, false
	}
	return m.visible[m.selectedIndex], true
}

func (m *Model) move(delta int) {
	m.selectedIndex += delta
	m.clampSelection()
}

// clampSelection keeps selectedIndex on a valid row and scrolls so it is drawn.
func (m *Model) clampSelection() {
	if m.selectedIndex >= len(m.visible) {
		m.selectedIndex = len(m.visible) - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
	rows := m.tableRows()
	if m.selectedIndex < m.offset {
		m.offset = m.selectedIndex
	}
	if m.selectedIndex >= m.offset+rows {
		m.offset = m.selectedIndex - rows + 1
	}
	if maxOff := len(m.visible) - rows; m.offset > maxOff {
		m.offset = max(maxOff, 0)
	}
}

func (m *Model) setStatus(kind statusKind, msg string) {
	m.statusKind, m.status = kind, msg
}

// needsTypedConfirmation reports whether killing a process of this tier
// requires typing its name rather than a plain y/N. Unknown tiers err toward
// the stricter flow.
func needsTypedConfirmation(tier string) bool {
	return tier != risk.Low
}

func (m *Model) openKillDialog(e inspector.PortEntry) {
	m.confirmingKill = true
	m.pendingKill = &e
	m.typedConfirmation = ""
	m.confirmErr = ""
}

func (m *Model) closeKillDialog() {
	m.confirmingKill = false
	m.pendingKill = nil
	m.typedConfirmation = ""
	m.confirmErr = ""
}

// pendingStillListed reports whether the process being confirmed is still in
// the latest scan; the dialog warns if it has vanished.
func (m Model) pendingStillListed() bool {
	if m.pendingKill == nil {
		return false
	}
	for _, e := range m.ports {
		if keyOf(e) == keyOf(*m.pendingKill) {
			return true
		}
	}
	return false
}
