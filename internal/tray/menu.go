// Package tray is the system tray companion: a notification-area icon whose
// menu lists killable ports and kills them with the same risk-tiered rules
// as the TUI. This file holds the platform-independent menu logic; the
// Windows integration lives in tray_windows.go and win32_windows.go.
package tray

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/kill"
	"github.com/rpratap2111/portfind/internal/risk"
)

// maxPortItems caps the ports listed in the menu; the TUI shows everything.
const maxPortItems = 12

// menuPorts picks the ports to list: only processes portfind could actually
// kill, most likely targets first (see targetRank), each group by port. It
// also reports how many ports were left out because they can never be killed
// or Windows denied access (hidden), and how many didn't fit (overflow).
func menuPorts(entries []inspector.PortEntry, limit int) (shown []inspector.PortEntry, hidden, overflow int) {
	for _, e := range entries {
		if kill.CheckAllowed(targetOf(e)) != nil {
			hidden++ // kernel or critical Windows process
			continue
		}
		if e.Command == "" {
			hidden++ // OpenProcess was denied, so a kill would be too
			continue
		}
		shown = append(shown, e)
	}
	sort.SliceStable(shown, func(i, j int) bool { // input is already in port order
		return targetRank(shown[i]) < targetRank(shown[j])
	})
	if len(shown) > limit {
		overflow = len(shown) - limit
		shown = shown[:limit]
	}
	return shown, hidden, overflow
}

// targetRank orders the menu by how likely a port is to be what the user
// wants to free: dev servers, then anything from one of their projects, then
// databases and other HIGH processes, and last unrecognized processes with no
// project (IDE helpers, launchers), which would otherwise crowd the menu.
func targetRank(e inspector.PortEntry) int {
	switch {
	case e.RiskTier == risk.Low:
		return 0
	case e.ProjectName != "":
		return 1
	case e.RiskTier == risk.High:
		return 2
	default:
		return 3
	}
}

func targetOf(e inspector.PortEntry) kill.Target {
	return kill.Target{PID: e.PID, Port: e.Port, Process: e.Process}
}

// menuLabel renders an entry as "node — :3000 (my-app)" with the risk tier
// right-aligned (text after a tab goes in the menu's accelerator column).
func menuLabel(e inspector.PortEntry) string {
	s := fmt.Sprintf("%s — :%d", e.Process, e.Port)
	if e.ProjectName != "" {
		s += " (" + e.ProjectName + ")"
	}
	return escapeMenuText(s) + "\t" + e.RiskTier
}

// escapeMenuText doubles '&' so Windows shows it literally instead of
// treating it as a keyboard-mnemonic marker, and flattens control characters.
func escapeMenuText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	return strings.ReplaceAll(s, "&", "&&")
}

// menuTooltip is the per-item hover text.
func menuTooltip(e inspector.PortEntry) string {
	return fmt.Sprintf("PID %d · up %s · %s risk\n%s", e.PID, inspector.FormatAge(e.AgeSeconds), e.RiskTier, e.Command)
}

// confirmText is the body of the native confirmation dialog shown before
// killing a MEDIUM or HIGH risk process.
func confirmText(e inspector.PortEntry) string {
	project := e.ProjectName
	if project == "" {
		project = "none detected"
	}
	reason := "portfind doesn't recognise this process, or it is a Windows service."
	if e.RiskTier == risk.High {
		reason = "This looks like a database, or a process started over SSH."
	}
	return fmt.Sprintf("Kill %s on port %d?\n\n"+
		"Project:\t%s\n"+
		"PID:\t%d (running for %s)\n"+
		"Command:\t%s\n\n"+
		"%s risk: %s",
		e.Process, e.Port, project, e.PID, inspector.FormatAge(e.AgeSeconds),
		truncate(e.Command, 200), e.RiskTier, reason)
}

// Notification texts. Windows caps the title at 63 and the body at 255
// characters; notify truncates, these just keep them short.
func killedTitle(e inspector.PortEntry) string {
	return fmt.Sprintf("Killed %s on :%d", e.Process, e.Port)
}

func killFailedTitle(e inspector.PortEntry) string {
	return fmt.Sprintf("Couldn't kill %s on :%d", e.Process, e.Port)
}

func killedBody(e inspector.PortEntry) string {
	if e.ProjectName == "" {
		return fmt.Sprintf("PID %d", e.PID)
	}
	return fmt.Sprintf("%s · PID %d", e.ProjectName, e.PID)
}

// fightLabel summarizes port fights for the menu, or "" if there are none.
func fightLabel(fights []history.Fight) string {
	if len(fights) == 0 {
		return ""
	}
	var ports []string
	for _, f := range fights {
		ports = append(ports, fmt.Sprintf(":%d (%d×)", f.Port, f.Kills))
	}
	return fmt.Sprintf("⚡ %s killed repeatedly in %dm: something keeps restarting it",
		strings.Join(ports, ", "), int(history.FightWindow.Minutes()))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
