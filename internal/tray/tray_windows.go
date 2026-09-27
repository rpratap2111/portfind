//go:build windows

package tray

import (
	"fmt"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"github.com/rpratap2111/portfind/assets"
	"github.com/rpratap2111/portfind/internal/history"
	"github.com/rpratap2111/portfind/internal/inspector"
	"github.com/rpratap2111/portfind/internal/kill"
	"github.com/rpratap2111/portfind/internal/risk"
	"github.com/rpratap2111/portfind/internal/scan"
)

// slot is one pre-created port menu item. systray can't remove items, so the
// menu has maxPortItems slots that are retitled and shown or hidden.
//
// Each port is a submenu: clicking the port itself does nothing, and killing
// takes a second, deliberate click on the Kill item beneath its details.
type slot struct {
	item                  *systray.MenuItem // "node — :3000 (my-app)   LOW ▸"
	pid, project, command *systray.MenuItem // read-only details
	killItem              *systray.MenuItem // "Kill node" / "Kill postgres…"
	entry                 inspector.PortEntry
	used                  bool
}

type app struct {
	ins  inspector.PortInspector
	hist *history.Store // nil if the database could not be opened
	hwnd uintptr        // systray's hidden window; 0 if not found

	mu    sync.Mutex // guards slots' entries and the menu items' state
	slots []*slot

	header, fight, empty, hidden, overflow *systray.MenuItem
	startup                                *systray.MenuItem // "Start with Windows" checkbox
}

// Run shows the tray icon and blocks until the user quits. It returns
// ErrAlreadyRunning if another instance is active.
func Run() error {
	release, err := singleInstance()
	if err != nil {
		return err
	}
	defer release()

	a := &app{ins: inspector.New()}
	var histErr error
	if path, err := history.DefaultPath(); err != nil {
		histErr = err
	} else if a.hist, err = history.Open(path); err != nil {
		histErr = err
	}

	systray.Run(func() { a.onReady(histErr) }, a.onExit)
	return nil
}

func (a *app) onReady(histErr error) {
	systray.SetIcon(assets.Icon)
	systray.SetTooltip("portfind")

	a.header = disabledItem("portfind")
	a.fight = hiddenItem()
	systray.AddSeparator()
	for i := 0; i < maxPortItems; i++ {
		// Sub-items must be added while the parent is visible (systray
		// converts it to a submenu then); hiding keeps the submenu attached.
		s := &slot{item: systray.AddMenuItem("", "")}
		s.pid = s.item.AddSubMenuItem("", "")
		s.project = s.item.AddSubMenuItem("", "")
		s.command = s.item.AddSubMenuItem("", "")
		for _, info := range []*systray.MenuItem{s.pid, s.project, s.command} {
			info.Disable()
		}
		s.killItem = s.item.AddSubMenuItem("", "")
		s.item.Hide()
		a.slots = append(a.slots, s)
		go a.watchSlot(s)
	}
	a.empty = hiddenItem()
	a.hidden = hiddenItem()
	a.overflow = hiddenItem()
	systray.AddSeparator()
	openTUI := systray.AddMenuItem("Open Terminal UI", "Open portfind in a new terminal window")
	a.startup = systray.AddMenuItemCheckbox("Start with Windows", "Start the portfind tray icon when you sign in", false)
	quit := systray.AddMenuItem("Quit", "Close the portfind tray icon")
	go a.watch(openTUI, func() {
		if err := launchTUI(); err != nil {
			a.notify("Couldn't open the terminal UI", err.Error(), notifyError)
		}
	})
	go a.watch(a.startup, a.toggleStartup)
	go a.watch(quit, systray.Quit)

	hwnd, err := findTrayWindow()
	if err == nil {
		a.hwnd = hwnd
		err = installClickHook(hwnd, a.refresh)
	}
	if err != nil {
		// Without the hook the menu still works, but only reflects the scan
		// from startup; say so rather than silently showing stale data.
		ShowError(fmt.Errorf("portfind can't refresh its menu when clicked (%v). The port list will only update when the tray app restarts.", err))
	}

	a.refresh()
	if histErr != nil {
		a.notify("portfind history unavailable", histErr.Error(), notifyWarning)
	}
}

func (a *app) onExit() {
	if a.hist != nil {
		a.hist.Close()
	}
}

// watch runs fn for every click on item. systray drops clicks that arrive
// while nobody is receiving, so the handler runs on its own goroutine and the
// loop goes straight back to listening.
func (a *app) watch(item *systray.MenuItem, fn func()) {
	for range item.ClickedCh {
		go fn()
	}
}

func (a *app) watchSlot(s *slot) {
	for range s.killItem.ClickedCh {
		a.mu.Lock()
		e, ok := s.entry, s.used
		a.mu.Unlock()
		if ok {
			go a.kill(e)
		}
	}
}

// refresh rescans and rebuilds the menu. It runs on the tray's UI thread
// right before the menu opens (see installClickHook), and once at startup.
func (a *app) refresh() {
	res, err := scan.Run(a.ins)

	a.mu.Lock()
	defer a.mu.Unlock()

	if err != nil {
		a.header.SetTitle(escapeMenuText("Scan failed: " + err.Error()))
		for _, s := range a.slots {
			s.used = false
			s.item.Hide()
		}
		for _, item := range []*systray.MenuItem{a.fight, a.empty, a.hidden, a.overflow} {
			item.Hide()
		}
		return
	}

	shown, hidden, overflow := menuPorts(res.Entries, len(a.slots))
	a.header.SetTitle(fmt.Sprintf("portfind · %d listening ports", len(res.Entries)))
	systray.SetTooltip(fmt.Sprintf("portfind: %d listening ports", len(res.Entries)))

	for i, s := range a.slots {
		if i >= len(shown) {
			s.used = false
			s.item.Hide()
			continue
		}
		s.entry, s.used = shown[i], true
		pid, project, command := portDetails(shown[i])
		s.item.SetTitle(menuLabel(shown[i]))
		s.pid.SetTitle(pid)
		s.project.SetTitle(project)
		s.command.SetTitle(command)
		s.killItem.SetTitle(killLabel(shown[i]))
		s.item.Show()
	}

	setInfo(a.empty, len(shown) == 0, "No ports you can kill right now")
	setInfo(a.hidden, hidden > 0, fmt.Sprintf("%d system or elevated ports not shown", hidden))
	setInfo(a.overflow, overflow > 0, fmt.Sprintf("…and %d more (Open Terminal UI to see all)", overflow))

	label := ""
	if a.hist != nil {
		fights, err := a.hist.PortFights(time.Now().Add(-history.FightWindow), history.FightThreshold)
		if err != nil {
			label = "History error: " + err.Error()
		} else {
			label = fightLabel(fights)
		}
	}
	setInfo(a.fight, label != "", escapeMenuText(label))
	a.syncStartupCheck()
}

// syncStartupCheck makes the checkbox match the registry, which the user can
// also change from Task Manager or the uninstaller.
func (a *app) syncStartupCheck() {
	on, err := autostartEnabled()
	if err != nil {
		a.startup.SetTitle(escapeMenuText("Start with Windows (unavailable: " + err.Error() + ")"))
		a.startup.Disable()
		return
	}
	if on {
		a.startup.Check()
	} else {
		a.startup.Uncheck()
	}
}

func (a *app) toggleStartup() {
	a.mu.Lock()
	want := !a.startup.Checked()
	a.mu.Unlock()

	if err := setAutostart(want); err != nil {
		a.notify("Couldn't change Start with Windows", err.Error(), notifyError)
		return
	}
	a.mu.Lock()
	a.syncStartupCheck()
	a.mu.Unlock()
	if want {
		a.notify("portfind will start with Windows", "The tray icon will appear when you sign in.", notifyInfo)
	} else {
		a.notify("portfind won't start with Windows", "Start it from the Start menu when you need it.", notifyInfo)
	}
}

// kill runs the risk-tiered kill flow for an entry picked from the menu:
// LOW kills straight away, MEDIUM and HIGH ask first. Every outcome ends in
// a notification.
func (a *app) kill(e inspector.PortEntry) {
	if e.RiskTier != risk.Low {
		ok, err := confirmDialog("portfind: confirm kill", confirmText(e), e.RiskTier == risk.High)
		if err != nil {
			a.notify(killFailedTitle(e), "Couldn't show the confirmation dialog: "+err.Error(), notifyError)
			return
		}
		if !ok {
			return
		}
	}

	if err := kill.Terminate(a.ins, targetOf(e)); err != nil {
		a.notify(killFailedTitle(e), err.Error(), notifyError)
		return
	}
	if a.hist != nil {
		err := a.hist.Record(history.Event{
			At: time.Now(), Port: e.Port, PID: e.PID, Process: e.Process,
			Project: e.ProjectName, KilledViaPortfind: true,
		})
		if err != nil {
			a.notify(killedTitle(e), killedBody(e)+" (not saved to history: "+err.Error()+")", notifyWarning)
			return
		}
	}
	a.notify(killedTitle(e), killedBody(e), notifyInfo)
}

// notify shows a toast, falling back to a dialog so a result is never lost.
func (a *app) notify(title, body string, kind notifyKind) {
	if err := showBalloon(a.hwnd, title, body, kind); err != nil {
		if kind == notifyInfo {
			ShowInfo(title + "\n\n" + body)
		} else {
			ShowError(fmt.Errorf("%s\n\n%s", title, body))
		}
	}
}

func disabledItem(title string) *systray.MenuItem {
	item := systray.AddMenuItem(title, "")
	item.Disable()
	return item
}

func hiddenItem() *systray.MenuItem {
	item := disabledItem("")
	item.Hide()
	return item
}

// setInfo shows a disabled informational item with title, or hides it.
func setInfo(item *systray.MenuItem, show bool, title string) {
	if !show {
		item.Hide()
		return
	}
	item.SetTitle(title)
	item.Show()
}
