package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"portfind/internal/history"
	"portfind/internal/inspector"
	"portfind/internal/risk"
)

// Tokyo Night palette.
var (
	colBg      = lipgloss.Color("#1a1b26")
	colFg      = lipgloss.Color("#c0caf5")
	colBorder  = lipgloss.Color("#7aa2f7")
	colAccent  = lipgloss.Color("#bb9af7")
	colSelBg   = lipgloss.Color("#283457")
	colHeader  = lipgloss.Color("#7dcfff")
	colMuted   = lipgloss.Color("#565f89")
	colLow     = lipgloss.Color("#9ece6a")
	colMedium  = lipgloss.Color("#e0af68")
	colHigh    = lipgloss.Color("#f7768e")
	colProject = colAccent
	colError   = colHigh
	colSuccess = colLow
	colInfo    = colMedium
)

// Every style carries an explicit background. Lip Gloss resets attributes
// between styled segments, so any unstyled gap would fall back to the
// terminal's own background and break the theme.
var (
	base         = lipgloss.NewStyle().Background(colBg).Foreground(colFg)
	boxStyle     = base.Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).BorderBackground(colBg)
	containerBox = boxStyle.Padding(1)
	// The table box pads vertically only; each row adds its own 1-cell side
	// margin in the row's background so the selection highlight spans the
	// full inner width, k9s-style.
	tableBox    = boxStyle.Padding(1, 0)
	searchLabel = base.Foreground(colAccent).Bold(true)
	cursorStyle = base.Foreground(colAccent)
	mutedStyle  = base.Foreground(colMuted)
)

// Vertical space used by everything except table rows: container border (2)
// and padding (2), search line and blank (2), table border (2), padding (2)
// and header (1), then blank, status and footer (3).
const chromeHeight = 14

const (
	defaultWidth  = 120
	defaultHeight = 30
	colGap        = "  "
)

const (
	footerText        = "↑/↓ nav  type to search  enter kill  tab history  ctrl+r refresh  ctrl+w warnings  esc quit"
	footerHistory     = "type to filter  tab/esc back to ports"
	footerWarnings    = "ctrl+w/esc back to ports"
	footerConfirmLow  = "y kill  n/enter cancel  esc cancel"
	footerConfirmName = "type the process name  enter confirm  esc cancel"
	dialogMaxWidth    = 76
)

func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	return w, h
}

// tableRows is how many data rows fit on screen.
func (m Model) tableRows() int {
	_, h := m.size()
	return max(h-chromeHeight, 1)
}

// View renders the whole screen.
func (m Model) View() string {
	w, h := m.size()
	innerW := max(w-4, 20) // container border + padding
	rowW := innerW - 2     // table border

	var body string
	switch {
	case m.showWarnings:
		body = m.viewWarnings(rowW)
	case m.showHistory:
		body = m.viewHistory(rowW)
	case len(m.visible) == 0:
		body = m.viewMessage(rowW, m.emptyMessage())
	default:
		body = m.viewTable(rowW)
	}

	lines := []string{
		m.viewSearch(innerW),
		blankLine(innerW, colBg),
		tableBox.Render(body),
		blankLine(innerW, colBg),
		m.viewStatus(innerW),
		fit(mutedStyle.Render(m.footer()), innerW, colBg),
	}
	screen := containerBox.Render(strings.Join(lines, "\n"))
	screen = lipgloss.Place(w, h, lipgloss.Left, lipgloss.Top, screen,
		lipgloss.WithWhitespaceBackground(colBg))

	if m.confirmingKill && m.pendingKill != nil {
		dialog := m.viewKillDialog(min(dialogMaxWidth, w-4))
		x := (w - lipgloss.Width(dialog)) / 2
		y := max((h-lipgloss.Height(dialog))/2, 0)
		screen = overlay(screen, dialog, x, y)
	}
	return screen
}

func (m Model) footer() string {
	switch {
	case m.showHistory:
		return footerHistory
	case m.showWarnings:
		return footerWarnings
	case !m.confirmingKill || m.pendingKill == nil:
		return footerText
	case needsTypedConfirmation(m.pendingKill.RiskTier):
		return footerConfirmName
	default:
		return footerConfirmLow
	}
}

func (m Model) viewSearch(width int) string {
	s := searchLabel.Render("Search: ") + base.Render(m.searchQuery) + cursorStyle.Render("█")
	hint := fightHint(m.fights)
	if hint == "" {
		return fit(s, width, colBg)
	}
	// Right-align the hint; drop it rather than crowd out the query.
	gap := width - ansi.StringWidth(s) - ansi.StringWidth(hint)
	if gap < 2 {
		return fit(s, width, colBg)
	}
	return s + base.Render(strings.Repeat(" ", gap)) + base.Foreground(colMedium).Render(hint)
}

// fightHint summarizes port fights in one quiet line, e.g.
// "⚡ :3000 killed 3× in 15m (node · demo-web-app): something keeps restarting it".
func fightHint(fights []history.Fight) string {
	window := fmt.Sprintf("%dm", int(history.FightWindow.Minutes()))
	switch len(fights) {
	case 0:
		return ""
	case 1:
		f := fights[0]
		who := f.Process
		if f.Project != "" {
			who += " · " + f.Project
		}
		return fmt.Sprintf("⚡ :%d killed %d× in %s (%s): something keeps restarting it", f.Port, f.Kills, window, who)
	default:
		var ports []string
		for _, f := range fights {
			ports = append(ports, fmt.Sprintf(":%d %d×", f.Port, f.Kills))
		}
		return fmt.Sprintf("⚡ %s killed in %s: something keeps restarting them", strings.Join(ports, ", "), window)
	}
}

func (m Model) viewStatus(width int) string {
	if m.scanErr != nil {
		msg := fmt.Sprintf("scan failed: %v (showing last good data)", m.scanErr)
		return fit(base.Foreground(colError).Render(msg), width, colBg)
	}
	if m.status != "" {
		col := map[statusKind]lipgloss.Color{statusInfo: colInfo, statusSuccess: colSuccess, statusError: colError}[m.statusKind]
		return fit(base.Foreground(col).Render(sanitize(m.status)), width, colBg)
	}

	parts := []string{fmt.Sprintf("%d ports", len(m.ports))}
	if m.searchQuery != "" {
		parts = append(parts, fmt.Sprintf("%d matching", len(m.visible)))
	}
	if rows := m.tableRows(); len(m.visible) > rows && !m.showWarnings && !m.showHistory {
		last := min(m.offset+rows, len(m.visible))
		parts = append(parts, fmt.Sprintf("rows %d–%d", m.offset+1, last))
	}
	if !m.lastScan.IsZero() {
		parts = append(parts, "refreshed "+m.lastScan.Format("15:04:05"))
	}
	s := mutedStyle.Render(strings.Join(parts, " · "))
	if n := len(m.warnings); n > 0 {
		s += mutedStyle.Render(" · ") + base.Foreground(colMedium).Render(fmt.Sprintf("%d warnings (ctrl+w)", n))
	}
	if m.histErr != nil {
		s += mutedStyle.Render(" · ") + base.Foreground(colError).Render("history error (ctrl+w)")
	}
	return fit(s, width, colBg)
}

func (m Model) emptyMessage() string {
	if len(m.ports) == 0 {
		return "No listening ports"
	}
	return "No matching ports"
}

// columns holds the width of each table column.
type columns struct {
	port, pid, project, process, age, risk, command int
}

// layoutColumns sizes columns from all ports, not just the filtered ones, so
// the table doesn't shift around while typing a search.
func layoutColumns(entries []inspector.PortEntry, width int) columns {
	c := columns{port: 5, pid: 6, age: 6, risk: 6}
	for _, e := range entries {
		c.project = max(c.project, ansi.StringWidth(orDash(e.ProjectName)))
		c.process = max(c.process, ansi.StringWidth(e.Process))
	}
	c.project = clamp(c.project, len("PROJECT"), 22)
	c.process = clamp(c.process, len("PROCESS"), 20)

	fixed := func() int {
		return c.port + c.pid + c.project + c.process + c.age + c.risk + 6*len(colGap)
	}
	if width-fixed() < 10 { // squeeze names before starving COMMAND
		c.project, c.process = min(c.project, 8), min(c.process, 8)
	}
	c.command = max(width-fixed(), 0)
	return c
}

func (m Model) viewTable(rowW int) string {
	contentW := rowW - 2 // 1-cell margin each side
	cols := layoutColumns(m.ports, contentW)

	hdr := base.Foreground(colHeader).Bold(true)
	header := []string{
		cell(hdr, "PORT", cols.port), cell(hdr, "PID", cols.pid),
		cell(hdr, "PROJECT", cols.project), cell(hdr, "PROCESS", cols.process),
		cell(hdr, "AGE", cols.age), cell(hdr, "RISK", cols.risk),
		cell(hdr, "COMMAND", cols.command),
	}
	lines := []string{row(header, rowW, colBg)}

	rows := m.tableRows()
	end := min(m.offset+rows, len(m.visible))
	for i := m.offset; i < end; i++ {
		lines = append(lines, renderEntry(m.visible[i], cols, rowW, i == m.selectedIndex))
	}
	for len(lines) < rows+1 { // fixed height keeps the footer from jumping
		lines = append(lines, blankLine(rowW, colBg))
	}
	return strings.Join(lines, "\n")
}

func renderEntry(e inspector.PortEntry, cols columns, rowW int, selected bool) string {
	bg := colBg
	if selected {
		bg = colSelBg
	}
	st := lipgloss.NewStyle().Background(bg).Foreground(colFg)
	cells := []string{
		cell(st, strconv.Itoa(e.Port), cols.port),
		cell(st, strconv.Itoa(e.PID), cols.pid),
		cell(st.Foreground(colProject), orDash(e.ProjectName), cols.project),
		cell(st, e.Process, cols.process),
		cell(st, formatAge(e.AgeSeconds), cols.age),
		cell(st.Foreground(riskColor(e.RiskTier)), e.RiskTier, cols.risk),
		cell(st, sanitize(e.Command), cols.command),
	}
	return row(cells, rowW, bg)
}

// row joins cells with gaps and a 1-cell margin on each side, all in bg,
// filling exactly width cells.
func row(cells []string, width int, bg lipgloss.Color) string {
	st := lipgloss.NewStyle().Background(bg)
	s := st.Render(" ") + strings.Join(cells, st.Render(colGap))
	return fit(s, width, bg)
}

// cell truncates text to w cells (with an ellipsis) and pads it to exactly w.
func cell(st lipgloss.Style, text string, w int) string {
	if w <= 0 {
		return ""
	}
	return st.Width(w).Render(ansi.Truncate(text, w, "…"))
}

func (m Model) viewMessage(rowW int, msg string) string {
	lines := []string{row([]string{mutedStyle.Render(msg)}, rowW, colBg)}
	for len(lines) < m.tableRows()+1 {
		lines = append(lines, blankLine(rowW, colBg))
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewWarnings(rowW int) string {
	hdr := base.Foreground(colHeader).Bold(true)
	title := fmt.Sprintf("WARNINGS (%d) · ctrl+w to close", len(m.warnings))
	lines := []string{row([]string{hdr.Render(title)}, rowW, colBg)}

	type item struct {
		text string
		col  lipgloss.Color
	}
	var items []item
	if m.histErr != nil {
		items = append(items, item{"history: " + m.histErr.Error(), colError})
	}
	for _, w := range m.warnings {
		items = append(items, item{w.Error(), colMedium})
	}

	rows := m.tableRows()
	for i, it := range items {
		if i == rows-1 && len(items) > rows {
			more := fmt.Sprintf("… and %d more", len(items)-i)
			lines = append(lines, row([]string{mutedStyle.Render(more)}, rowW, colBg))
			break
		}
		text := ansi.Truncate(sanitize(it.text), rowW-2, "…")
		lines = append(lines, row([]string{base.Foreground(it.col).Render(text)}, rowW, colBg))
	}
	if len(items) == 0 {
		lines = append(lines, row([]string{mutedStyle.Render("No warnings from the last scan")}, rowW, colBg))
	}
	return padRows(lines, rows+1, rowW)
}

// viewHistory lists recent port events, newest first, filtered by the search.
func (m Model) viewHistory(rowW int) string {
	if m.hist == nil {
		msg := "History is unavailable"
		if m.histErr != nil {
			msg += ": " + m.histErr.Error()
		}
		return m.viewMessage(rowW, msg)
	}

	terms := strings.Fields(strings.ToLower(m.searchQuery))
	var events []history.Event
	for _, e := range m.historyRows {
		if matchesAll(inspector.PortEntry{Port: e.Port, Process: e.Process, ProjectName: e.Project}, terms) {
			events = append(events, e)
		}
	}
	if len(events) == 0 {
		msg := "No history yet: processes are logged when they leave a port"
		if len(m.historyRows) > 0 {
			msg = "No matching history"
		}
		return m.viewMessage(rowW, msg)
	}

	const timeW, portW, pidW, whatW = 14, 5, 6, 18
	projW := clamp(maxWidth(events, func(e history.Event) string { return orDash(e.Project) }), len("PROJECT"), 22)
	procW := clamp(maxWidth(events, func(e history.Event) string { return e.Process }), len("PROCESS"), 20)

	hdr := base.Foreground(colHeader).Bold(true)
	lines := []string{row([]string{
		cell(hdr, "WHEN", timeW), cell(hdr, "PORT", portW), cell(hdr, "PID", pidW),
		cell(hdr, "PROJECT", projW), cell(hdr, "PROCESS", procW), cell(hdr, "WHAT", whatW),
	}, rowW, colBg)}

	rows := m.tableRows()
	now := time.Now()
	for i, e := range events {
		if i == rows-1 && len(events) > rows {
			more := fmt.Sprintf("… %d older events", len(events)-i)
			lines = append(lines, row([]string{mutedStyle.Render(more)}, rowW, colBg))
			break
		}
		what, whatCol := "exited", colMuted
		if e.KilledViaPortfind {
			what, whatCol = "killed by portfind", colHigh
		}
		lines = append(lines, row([]string{
			cell(mutedStyle, formatWhen(e.At, now), timeW),
			cell(base, strconv.Itoa(e.Port), portW),
			cell(base, strconv.Itoa(e.PID), pidW),
			cell(base.Foreground(colProject), orDash(e.Project), projW),
			cell(base, e.Process, procW),
			cell(base.Foreground(whatCol), what, whatW),
		}, rowW, colBg))
	}
	return padRows(lines, rows+1, rowW)
}

// formatWhen shows the time of day for today's events and the date otherwise.
func formatWhen(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	if y1 == y2 && m1 == m2 && d1 == d2 {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func maxWidth(events []history.Event, field func(history.Event) string) int {
	w := 0
	for _, e := range events {
		w = max(w, ansi.StringWidth(field(e)))
	}
	return w
}

// padRows pads lines with blank rows to a fixed height so the layout below
// the table doesn't jump.
func padRows(lines []string, height, width int) string {
	for len(lines) < height {
		lines = append(lines, blankLine(width, colBg))
	}
	return strings.Join(lines, "\n")
}

// fit truncates or pads a rendered line to exactly width cells, padding in bg.
func fit(s string, width int, bg lipgloss.Color) string {
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width, "")
	}
	return s + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width-w))
}

func blankLine(width int, bg lipgloss.Color) string {
	return lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width))
}

func riskColor(tier string) lipgloss.Color {
	switch tier {
	case risk.Low:
		return colLow
	case risk.High:
		return colHigh
	default:
		return colMedium
	}
}

// formatAge renders seconds compactly, e.g. "45s", "12m", "2h15m", "3d4h".
// Negative values mean unknown.
func formatAge(sec int64) string {
	switch {
	case sec < 0:
		return "?"
	case sec < 60:
		return fmt.Sprintf("%ds", sec)
	case sec < 3600:
		return fmt.Sprintf("%dm", sec/60)
	case sec < 86400:
		return fmt.Sprintf("%dh%dm", sec/3600, sec%3600/60)
	default:
		return fmt.Sprintf("%dd%dh", sec/86400, sec%86400/3600)
	}
}

// sanitize flattens control characters so a hostile or odd command line
// can't break the layout or inject escape sequences.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// viewKillDialog renders the confirmation box for pendingKill, bordered in
// the target's risk colour.
func (m Model) viewKillDialog(width int) string {
	e := *m.pendingKill
	rc := riskColor(e.RiskTier)
	contentW := max(width-2-4, 10) // border, horizontal padding
	line := func(s string) string { return fit(s, contentW, colBg) }

	title := base.Bold(true).Render(fmt.Sprintf("Kill %s on :%d?", e.Process, e.Port))
	typed := needsTypedConfirmation(e.RiskTier)
	if !typed {
		title += base.Render(" ") + base.Foreground(colAccent).Bold(true).Render("[y/N]")
	}

	project := mutedStyle.Render("no project")
	if e.ProjectName != "" {
		project = base.Foreground(colProject).Render(e.ProjectName)
	}
	sep := mutedStyle.Render(" · ")
	details := mutedStyle.Render(fmt.Sprintf("PID %d", e.PID)) + sep + project + sep +
		mutedStyle.Render("up "+formatAge(e.AgeSeconds)) + sep +
		base.Foreground(rc).Bold(true).Render(e.RiskTier)

	lines := []string{
		line(title),
		line(details),
		line(mutedStyle.Render(ansi.Truncate(sanitize(orDash(e.Command)), contentW, "…"))),
	}
	if !m.pendingStillListed() {
		lines = append(lines, line(base.Foreground(colMedium).Render("⚠ No longer in the port list; it may have exited.")))
	}
	lines = append(lines, blankLine(contentW, colBg))

	if typed {
		prompt := fmt.Sprintf("%s risk: type %q to confirm", e.RiskTier, e.Process)
		input := base.Foreground(colAccent).Render("› ") + base.Render(sanitize(m.typedConfirmation)) + cursorStyle.Render("█")
		errLine := blankLine(contentW, colBg)
		if m.confirmErr != "" {
			errLine = line(base.Foreground(colError).Render(sanitize(m.confirmErr)))
		}
		lines = append(lines,
			line(base.Foreground(rc).Render(prompt)),
			line(input),
			errLine,
			line(mutedStyle.Render("enter confirm · esc cancel")),
		)
	} else {
		lines = append(lines, line(mutedStyle.Render("y kill · n / enter / esc cancel")))
	}

	box := base.Border(lipgloss.RoundedBorder()).BorderForeground(rc).BorderBackground(colBg).Padding(1, 2)
	return box.Render(strings.Join(lines, "\n"))
}

// overlay draws fg on top of bg with its top-left corner at cell (x, y).
// Both are multi-line strings that may contain ANSI styling.
func overlay(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	for i, fl := range strings.Split(fg, "\n") {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		line := bgLines[row]
		left := ansi.Truncate(line, x, "")
		right := ansi.TruncateLeft(line, x+ansi.StringWidth(fl), "")
		// Reset between segments so no style bleeds across the seams.
		bgLines[row] = left + ansi.ResetStyle + fl + ansi.ResetStyle + right
	}
	return strings.Join(bgLines, "\n")
}
