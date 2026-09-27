//go:build windows

package tray

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These mirror internals of github.com/getlantern/systray v1.2.2
// (systray_windows.go): its hidden window's class, its tray icon's ID and the
// callback message the icon sends on mouse events. go.mod pins that version;
// re-check these if it is ever upgraded.
const (
	systrayWindowClass = "SystrayClass"
	systrayIconID      = 100
	systrayCallbackMsg = 0x0400 + 1 // WM_USER + 1

	wmLButtonUp = 0x0202
	wmRButtonUp = 0x0205
)

var gwlpWndProc = ^uintptr(3) // GWLP_WNDPROC (-4)

var (
	modUser32  = windows.NewLazySystemDLL("user32.dll")
	modShell32 = windows.NewLazySystemDLL("shell32.dll")

	procFindWindowExW     = modUser32.NewProc("FindWindowExW")
	procGetWindowLongPtrW = modUser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW = modUser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW   = modUser32.NewProc("CallWindowProcW")
	procShellNotifyIconW  = modShell32.NewProc("Shell_NotifyIconW")
)

// findTrayWindow returns the hidden window systray created in this process.
// Other apps built on the same library have windows of the same class, so
// matches are filtered by process ID.
func findTrayWindow() (uintptr, error) {
	if err := procFindWindowExW.Find(); err != nil {
		return 0, err
	}
	class, err := windows.UTF16PtrFromString(systrayWindowClass)
	if err != nil {
		return 0, err
	}
	self := windows.GetCurrentProcessId()
	var after uintptr
	for {
		h, _, _ := procFindWindowExW.Call(0, after, uintptr(unsafe.Pointer(class)), 0)
		if h == 0 {
			return 0, errors.New("the tray icon's window was not found")
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(windows.HWND(h), &pid); err == nil && pid == self {
			return h, nil
		}
		after = h
	}
}

// Click hook state. There is one tray icon per process, so package-level
// state is enough; NewCallback slots are never freed, so create one.
var (
	origWndProc  atomic.Uintptr
	beforeMenu   atomic.Pointer[func()]
	hookCallback = windows.NewCallback(hookWndProc)
)

// hookWndProc runs on the tray's UI thread for every message to its window.
// On a click on the icon it calls beforeMenu (which rescans and rebuilds the
// menu) and then lets systray show the menu as usual.
func hookWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == systrayCallbackMsg && (lParam == wmLButtonUp || lParam == wmRButtonUp) {
		if fn := beforeMenu.Load(); fn != nil {
			(*fn)()
		}
	}
	r, _, _ := procCallWindowProcW.Call(origWndProc.Load(), hwnd, msg, wParam, lParam)
	return r
}

// installClickHook subclasses the tray window so fn runs just before the
// menu opens. Because the menu is only rebuilt there, it never changes while
// it is on screen: the item you click is the process you saw.
func installClickHook(hwnd uintptr, fn func()) error {
	for _, p := range []*windows.LazyProc{procGetWindowLongPtrW, procSetWindowLongPtrW, procCallWindowProcW} {
		if err := p.Find(); err != nil {
			return err
		}
	}
	orig, _, err := procGetWindowLongPtrW.Call(hwnd, gwlpWndProc)
	if orig == 0 {
		return fmt.Errorf("GetWindowLongPtr: %w", err)
	}
	// Publish the original procedure before the hook can possibly run.
	origWndProc.Store(orig)
	beforeMenu.Store(&fn)
	if prev, _, err := procSetWindowLongPtrW.Call(hwnd, gwlpWndProc, hookCallback); prev == 0 {
		return fmt.Errorf("SetWindowLongPtr: %w", err)
	}
	return nil
}

// notifyIconData mirrors NOTIFYICONDATAW.
type notifyIconData struct {
	Size             uint32
	Wnd              windows.Handle
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State, StateMask uint32
	Info             [256]uint16
	Timeout, Version uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GuidItem         windows.GUID
	BalloonIcon      windows.Handle
}

type notifyKind uint32

const (
	notifyInfo    notifyKind = 0x1 // NIIF_INFO
	notifyWarning notifyKind = 0x2 // NIIF_WARNING
	notifyError   notifyKind = 0x3 // NIIF_ERROR
)

// showBalloon shows a notification from the tray icon. Windows 10 and 11
// present it as a toast. Only NIF_INFO is set, so the icon and tooltip that
// systray manages are left alone.
func showBalloon(hwnd uintptr, title, body string, kind notifyKind) error {
	const (
		nimModify = 0x1
		nifInfo   = 0x10
	)
	if hwnd == 0 {
		return errors.New("tray window unknown")
	}
	if err := procShellNotifyIconW.Find(); err != nil {
		return err
	}
	nid := notifyIconData{Wnd: windows.Handle(hwnd), ID: systrayIconID, Flags: nifInfo, InfoFlags: uint32(kind)}
	nid.Size = uint32(unsafe.Sizeof(nid))
	copyUTF16(nid.InfoTitle[:], title)
	copyUTF16(nid.Info[:], body)
	if r, _, err := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid))); r == 0 {
		return fmt.Errorf("Shell_NotifyIcon: %w", err)
	}
	return nil
}

// copyUTF16 copies s into a fixed NUL-terminated buffer, truncating to fit.
func copyUTF16(dst []uint16, s string) {
	u := windows.StringToUTF16(s) // includes the terminating NUL
	if len(u) > len(dst) {
		u = append(u[:len(dst)-2], '…', 0)
	}
	copy(dst, u)
}

// confirmDialog asks a yes/no question in a native dialog. No is the default
// button, so a stray Enter never kills anything.
func confirmDialog(caption, text string, high bool) (bool, error) {
	flags := uint32(windows.MB_YESNO | windows.MB_DEFBUTTON2 | windows.MB_SETFOREGROUND | windows.MB_TOPMOST)
	if high {
		flags |= windows.MB_ICONERROR
	} else {
		flags |= windows.MB_ICONWARNING
	}
	ret, err := messageBox(caption, text, flags)
	if ret == 0 {
		return false, fmt.Errorf("MessageBox: %w", err)
	}
	const idYes = 6 // IDYES
	return ret == idYes, nil
}

// ShowError reports an error in a native dialog; the tray app has no console.
func ShowError(err error) {
	messageBox("portfind", err.Error(), windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

// ShowInfo shows an informational native dialog.
func ShowInfo(msg string) {
	messageBox("portfind", msg, windows.MB_OK|windows.MB_ICONINFORMATION|windows.MB_SETFOREGROUND)
}

func messageBox(caption, text string, flags uint32) (int32, error) {
	t, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, err
	}
	c, err := windows.UTF16PtrFromString(caption)
	if err != nil {
		return 0, err
	}
	return windows.MessageBox(0, t, c, flags)
}

// launchTUI opens the terminal UI in a new console window. It prefers the
// portfind.exe next to the tray executable (how releases ship) and falls
// back to PATH.
func launchTUI() error {
	path := ""
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), "portfind.exe")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	}
	if path == "" {
		found, err := exec.LookPath("portfind")
		if err != nil {
			return errors.New("portfind.exe was not found next to the tray app or on PATH")
		}
		path = found
	}
	return startInNewConsole(path)
}

// startInNewConsole runs path in a new console window. It calls CreateProcess
// directly because os/exec always passes explicit std handles (the null device
// when unset), which would leave the TUI drawing into nothing instead of into
// its new console.
func startInNewConsole(path string) error {
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{path}))
	if err != nil {
		return err
	}
	var dir *uint16
	if home, err := os.UserHomeDir(); err == nil {
		dir, _ = windows.UTF16PtrFromString(home)
	}
	si := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(nil, cmdline, nil, nil, false,
		windows.CREATE_NEW_CONSOLE|windows.CREATE_UNICODE_ENVIRONMENT, nil, dir, &si, &pi); err != nil {
		return fmt.Errorf("start %s: %w", path, err)
	}
	windows.CloseHandle(pi.Thread)
	windows.CloseHandle(pi.Process)
	return nil
}

// ErrAlreadyRunning is returned by Run when another tray instance owns the icon.
var ErrAlreadyRunning = errors.New("portfind is already running in the notification area")

// singleInstance holds a named mutex for the life of the process so a second
// launch exits instead of adding a duplicate icon.
func singleInstance() (release func(), err error) {
	name, err := windows.UTF16PtrFromString(`Local\portfind-tray`)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(h)
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, fmt.Errorf("CreateMutex: %w", err)
	}
	return func() { windows.CloseHandle(h) }, nil
}
