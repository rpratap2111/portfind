package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/kill"
)

// Update handles input, the refresh tick, scan results and kill results.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampSelection()
		return m, nil

	case tickMsg:
		// Ignore stale ticks (a manual refresh restarts the chain) and don't
		// stack scans if one is still running.
		if msg.id != m.tickID || m.scanning {
			return m, nil
		}
		m.scanning = true
		return m, m.scanCmd()

	case scanDoneMsg:
		m.scanning = false
		m.applyScan(msg.res, msg.err, msg.at)
		m.tickID++
		return m, tickCmd(m.tickID)

	case killDoneMsg:
		m.killing = false
		t := msg.target
		if msg.err != nil {
			m.setStatus(statusError, "Kill failed: "+msg.err.Error())
		} else {
			m.markKilled(t.PID) // logged to history when the rescan sees it gone
			m.setStatus(statusSuccess, fmt.Sprintf("Killed %s (PID %d) on :%d", t.Process, t.PID, t.Port))
		}
		// Rescan right away so the row disappears without waiting for the tick.
		if !m.scanning {
			m.scanning = true
			return m, m.scanCmd()
		}
		return m, nil

	case tea.KeyMsg:
		if m.confirmingKill {
			return m.handleConfirmKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey implements the always-typing model: printable keys edit the
// search, so every command is on a non-printable key.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""

	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		if m.showHistory || m.showWarnings { // back out of a side view first
			m.showHistory, m.showWarnings = false, false
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyUp:
		m.move(-1)
	case tea.KeyDown:
		m.move(1)

	case tea.KeyTab:
		m.showHistory = !m.showHistory
		m.showWarnings = false
		if m.showHistory {
			m.loadHistoryRows()
		}

	case tea.KeyEnter:
		e, ok := m.selected()
		switch {
		case m.showHistory || m.showWarnings:
			m.setStatus(statusInfo, "Press tab or esc to return to the port list to kill")
		case !ok:
		case m.killing:
			m.setStatus(statusInfo, "A kill is already in progress")
		default:
			m.showWarnings = false
			m.openKillDialog(e)
		}

	case tea.KeyCtrlR:
		if !m.scanning {
			m.scanning = true
			return m, m.scanCmd()
		}

	case tea.KeyCtrlW:
		m.showWarnings = !m.showWarnings
		m.showHistory = false

	case tea.KeyBackspace:
		if q := []rune(m.searchQuery); len(q) > 0 {
			m.setQuery(string(q[:len(q)-1]))
		}

	case tea.KeyRunes, tea.KeySpace:
		if !msg.Alt {
			m.setQuery(m.searchQuery + string(msg.Runes))
		}
	}
	return m, nil
}

// handleConfirmKey drives the kill dialog. LOW risk is a plain [y/N] where
// anything but y cancels; MEDIUM and HIGH require typing the process name.
// Esc always cancels the dialog rather than quitting.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		return m.cancelKill()
	}

	target := *m.pendingKill
	if !needsTypedConfirmation(target.RiskTier) {
		if msg.Type == tea.KeyEnter {
			return m.cancelKill() // [y/N]: Enter takes the default, No
		}
		if msg.Type == tea.KeyRunes {
			switch string(msg.Runes) {
			case "y", "Y":
				return m.startKill()
			case "n", "N":
				return m.cancelKill()
			}
		}
		return m, nil // other keys are ignored, never treated as yes
	}

	switch msg.Type {
	case tea.KeyEnter:
		typed := strings.TrimSpace(m.typedConfirmation)
		if strings.EqualFold(typed, target.Process) {
			return m.startKill()
		}
		m.confirmErr = fmt.Sprintf("%q does not match %q", typed, target.Process)
	case tea.KeyBackspace:
		if r := []rune(m.typedConfirmation); len(r) > 0 {
			m.typedConfirmation = string(r[:len(r)-1])
		}
		m.confirmErr = ""
	case tea.KeyRunes, tea.KeySpace:
		if !msg.Alt {
			m.typedConfirmation += string(msg.Runes)
			m.confirmErr = ""
		}
	}
	return m, nil
}

func (m Model) cancelKill() (tea.Model, tea.Cmd) {
	m.setStatus(statusInfo, fmt.Sprintf("Kill cancelled: %s on :%d left running", m.pendingKill.Process, m.pendingKill.Port))
	m.closeKillDialog()
	return m, nil
}

// startKill closes the dialog and runs the kill in the background; the
// result arrives as a killDoneMsg.
func (m Model) startKill() (tea.Model, tea.Cmd) {
	e := *m.pendingKill
	t := kill.Target{PID: e.PID, Port: e.Port, Process: e.Process}
	m.closeKillDialog()
	m.killing = true
	m.setStatus(statusInfo, fmt.Sprintf("Killing %s (PID %d) on :%d…", t.Process, t.PID, t.Port))
	run := m.kill
	return m, func() tea.Msg { return killDoneMsg{target: t, err: run(t)} }
}
