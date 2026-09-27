package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/kill"
)

// withFakeKill makes m record kill calls instead of killing anything.
func withFakeKill(m *Model, err error) *[]kill.Target {
	var calls []kill.Target
	m.kill = func(t kill.Target) error {
		calls = append(calls, t)
		return err
	}
	return &calls
}

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// runCmd executes cmd and feeds its message back, returning the follow-up.
func (m *Model) runCmd(t *testing.T, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	return m.send(t, cmd())
}

func tiered(port, pid int, process, tier string) inspector.PortEntry {
	e := entry(port, pid, process, "")
	e.RiskTier = tier
	return e
}

func TestLowRiskKillWithY(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{tiered(8899, 42, "python", "LOW")})
	calls := withFakeKill(m, nil)

	m.send(t, key(tea.KeyEnter))
	if !m.confirmingKill {
		t.Fatal("Enter did not open the kill dialog")
	}
	if !strings.Contains(m.View(), "Kill python on :8899? [y/N]") {
		t.Fatal("LOW dialog prompt missing from view")
	}
	cmd := m.send(t, runes("y"))
	if m.confirmingKill || !m.killing {
		t.Fatal("y should close the dialog and start killing")
	}
	rescan := m.runCmd(t, cmd)
	if len(*calls) != 1 || (*calls)[0] != (kill.Target{PID: 42, Port: 8899, Process: "python"}) {
		t.Fatalf("kill calls = %+v", *calls)
	}
	if m.statusKind != statusSuccess || !strings.Contains(m.status, "Killed python (PID 42) on :8899") {
		t.Fatalf("status = %q (%v)", m.status, m.statusKind)
	}
	if rescan == nil || !m.scanning {
		t.Fatal("a successful kill should trigger an immediate rescan")
	}
}

func TestLowRiskDefaultsToNo(t *testing.T) {
	for _, k := range []tea.KeyMsg{key(tea.KeyEnter), runes("n"), runes("N"), key(tea.KeyEsc)} {
		m := newTestModel(t, []inspector.PortEntry{tiered(8899, 42, "python", "LOW")})
		calls := withFakeKill(m, nil)
		m.send(t, key(tea.KeyEnter))
		if cmd := m.send(t, k); cmd != nil {
			t.Fatalf("%v: returned a command; Esc/n/Enter must only cancel", k)
		}
		if m.confirmingKill || len(*calls) != 0 {
			t.Fatalf("%v: dialog open=%v kills=%d, want cancelled", k, m.confirmingKill, len(*calls))
		}
	}
}

func TestLowRiskIgnoresOtherKeys(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{tiered(8899, 42, "python", "LOW")})
	calls := withFakeKill(m, nil)
	m.send(t, key(tea.KeyEnter))
	for _, k := range []tea.KeyMsg{runes("x"), runes("yes"), key(tea.KeySpace), key(tea.KeyDown)} {
		m.send(t, k)
	}
	if !m.confirmingKill || len(*calls) != 0 || m.searchQuery != "" {
		t.Fatalf("open=%v kills=%d query=%q; stray keys must not kill or search", m.confirmingKill, len(*calls), m.searchQuery)
	}
}

func TestTypedConfirmationForMediumAndHigh(t *testing.T) {
	for _, tier := range []string{"MEDIUM", "HIGH", ""} {
		m := newTestModel(t, []inspector.PortEntry{tiered(6399, 7, "redis-server", tier)})
		calls := withFakeKill(m, nil)
		m.send(t, key(tea.KeyEnter))

		m.send(t, runes("y")) // y/N does not apply here
		if len(*calls) != 0 || !m.confirmingKill {
			t.Fatalf("%s: plain y must not kill", tier)
		}
		m.send(t, key(tea.KeyBackspace))

		m.send(t, runes("redis"))
		m.send(t, key(tea.KeyEnter))
		if len(*calls) != 0 || m.confirmErr == "" {
			t.Fatalf("%s: partial name must be rejected with an error", tier)
		}

		m.send(t, runes("-SERVER")) // case-insensitive
		if m.confirmErr != "" {
			t.Fatalf("%s: typing should clear the mismatch error", tier)
		}
		m.runCmd(t, m.send(t, key(tea.KeyEnter)))
		if len(*calls) != 1 {
			t.Fatalf("%s: correct name should kill once, got %d", tier, len(*calls))
		}
	}
}

func TestEscCancelsDialogWithoutQuitting(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{tiered(5432, 9, "postgres", "HIGH")})
	m.send(t, key(tea.KeyEnter))
	m.send(t, runes("postg"))
	if cmd := m.send(t, key(tea.KeyEsc)); cmd != nil {
		t.Fatal("Esc in the dialog must not quit")
	}
	if m.confirmingKill || m.typedConfirmation != "" {
		t.Fatal("Esc should close the dialog and clear typed text")
	}
}

func TestRefreshDuringDialogKeepsTarget(t *testing.T) {
	m := newTestModel(t,
		[]inspector.PortEntry{tiered(3000, 1, "node", "LOW"), tiered(8899, 2, "python", "LOW")},
		[]inspector.PortEntry{tiered(8899, 2, "python", "LOW")}, // node vanished
	)
	calls := withFakeKill(m, nil)
	m.send(t, key(tea.KeyEnter)) // node selected
	m.refresh(t)
	if m.pendingKill.Process != "node" || m.pendingStillListed() {
		t.Fatalf("pending=%+v stillListed=%v; target must not change on refresh", m.pendingKill, m.pendingStillListed())
	}
	if !strings.Contains(m.View(), "No longer in the port list") {
		t.Fatal("dialog should warn that the target vanished")
	}
	m.runCmd(t, m.send(t, runes("y")))
	if (*calls)[0].Process != "node" {
		t.Fatalf("killed %+v, want the node target", (*calls)[0])
	}
}

func TestKillErrorIsShown(t *testing.T) {
	m := newTestModel(t, []inspector.PortEntry{tiered(8899, 42, "python", "LOW")})
	withFakeKill(m, errors.New("OpenProcess: Access is denied."))
	m.send(t, key(tea.KeyEnter))
	m.runCmd(t, m.send(t, runes("y")))
	if m.statusKind != statusError || !strings.Contains(m.status, "Access is denied") {
		t.Fatalf("status = %q (%v); kill errors must be surfaced", m.status, m.statusKind)
	}
}
