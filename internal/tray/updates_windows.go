//go:build windows

package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/systray"

	"github.com/rpratap2111/portfind/internal/update"
)

const (
	firstUpdateCheck = 30 * time.Second // let sign-in settle first
	updateCheckEvery = 6 * time.Hour
	// Opening the menu also triggers a check if the last successful one is
	// older than this, so a new release shows up soon after it's published
	// rather than up to updateCheckEvery later.
	recheckOnOpen = 15 * time.Minute
)

// RestartError is returned by Run after "Restart to update" installed a new
// version: the caller should start Path with Args.
type RestartError struct {
	Path string
	Args []string
}

func (e *RestartError) Error() string { return "restart to finish updating" }

// displayVersion adds the "v": "1.2.0" -> "v1.2.0"; "dev" stays "dev".
func displayVersion(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// updater returns an Updater for this install. It targets portfind.exe next
// to the tray: that's the program --version can safely be run on (running
// the tray exe would start another tray), and the update replaces every
// portfind executable in the folder, the tray included.
func (a *app) updater() (*update.Updater, error) {
	u, err := update.New(a.opts.Version)
	if err != nil {
		return nil, err
	}
	u.Exe = filepath.Join(filepath.Dir(u.Exe), "portfind.exe")
	if _, err := os.Stat(u.Exe); err != nil {
		return nil, fmt.Errorf("portfind.exe isn't next to the tray app (%s), so it can't be updated in place; reinstall with the installer instead", filepath.Dir(u.Exe))
	}
	return u, nil
}

// checkForUpdates looks for a newer release shortly after start and then
// periodically. Development builds never check.
func (a *app) checkForUpdates() {
	if a.opts.Version == "dev" || a.opts.Version == "" {
		return
	}
	time.Sleep(firstUpdateCheck)
	for {
		a.checkOnce()
		time.Sleep(updateCheckEvery)
	}
}

// checkIfStale starts a background check when the menu is opened and the last
// successful check is older than recheckOnOpen. It never blocks the menu; the
// result shows as a notification and in the menu the next time it opens.
func (a *app) checkIfStale() {
	if a.opts.Version == "dev" || a.opts.Version == "" {
		return
	}
	if time.Since(time.Unix(a.lastCheck.Load(), 0)) >= recheckOnOpen {
		go a.checkOnce()
	}
}

func (a *app) checkOnce() {
	if !a.checking.CompareAndSwap(false, true) {
		return // a check is already in flight
	}
	defer a.checking.Store(false)

	u, err := update.New(a.opts.Version)
	if err != nil {
		a.showUpdateProblem(err)
		return
	}
	latest, newer, err := u.Check()
	if err != nil {
		// Usually just offline. Say so in the menu rather than with a toast
		// every six hours; the next check may well succeed.
		a.showUpdateProblem(err)
		return
	}
	a.lastCheck.Store(time.Now().Unix())

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updating {
		return
	}
	if !newer {
		a.latest = ""
		a.updateInfo.Hide()
		a.updateNow.Hide()
		return
	}
	announce := a.latest != latest
	a.latest = latest
	a.updateInfo.SetTitle(fmt.Sprintf("portfind %s is available", latest))
	a.updateInfo.Show()
	a.updateNow.SetTitle("Restart to update")
	a.updateNow.Enable()
	a.updateNow.Show()
	if announce {
		go a.notify("portfind "+latest+" is available", "Click the tray icon, then Restart to update.", notifyInfo)
	}
}

// showUpdateProblem notes a failed background check in the menu, unless an
// update is already on offer.
func (a *app) showUpdateProblem(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.latest != "" || a.updating {
		return
	}
	a.updateInfo.SetTitle(escapeMenuText("Couldn't check for updates: " + truncate(err.Error(), 60)))
	a.updateInfo.Show()
}

// installUpdate runs when "Restart to update" is clicked: it installs the new
// version (verified, as for `portfind --update`) and then quits so main can
// start the new tray.
func (a *app) installUpdate() {
	a.mu.Lock()
	if a.updating || a.latest == "" {
		a.mu.Unlock()
		return
	}
	a.updating = true
	target := a.latest
	a.updateInfo.SetTitle(fmt.Sprintf("Downloading portfind %s…", target))
	a.updateNow.Disable()
	a.mu.Unlock()

	res, err := a.install()
	if err != nil {
		a.mu.Lock()
		a.updating = false
		a.updateInfo.SetTitle(fmt.Sprintf("portfind %s is available", target))
		a.updateNow.Enable()
		a.mu.Unlock()
		a.notify("Update failed", err.Error(), notifyError)
		return
	}
	if !res.Updated { // someone else updated in the meantime
		a.mu.Lock()
		a.updating, a.latest = false, ""
		a.updateInfo.Hide()
		a.updateNow.Hide()
		a.mu.Unlock()
		return
	}

	dir := filepath.Dir(res.Path)
	a.restart = &RestartError{
		Path: filepath.Join(dir, "portfind-tray.exe"),
		Args: []string{"--updated-from", a.opts.Version},
	}
	systray.Quit()
}

type installed struct {
	Updated bool
	Path    string // portfind.exe that was updated
}

func (a *app) install() (installed, error) {
	u, err := a.updater()
	if err != nil {
		return installed{}, err
	}
	res, err := u.Run()
	if err != nil {
		return installed{}, err
	}
	return installed{Updated: res.Updated, Path: u.Exe}, nil
}
