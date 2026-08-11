//go:build windows

// Package tray puts an icon in the Windows notification area so kisaf can run
// quietly in the background and still be one click away.
//
// This talks to user32/shell32 directly through syscall rather than pulling in
// a tray library: it is a few hundred lines, it needs no cgo, and it keeps the
// promise that the whole program is a single dependency-free binary.
package tray

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessage         = user32.NewProc("PostMessageW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmApp     = 0x8000

	wmTrayCallback   = wmApp + 1
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonUp      = 0x0205
	wmContextMenuKey = 0x007B // WM_CONTEXTMENU, sent by the keyboard menu key

	nimAdd    = 0x0
	nimDelete = 0x2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	imageIcon      = 1
	lrLoadFromFile = 0x0010
	lrDefaultSize  = 0x0040
	idiApplication = 32512

	mfString    = 0x0000
	mfSeparator = 0x0800

	tpmRightButton = 0x0002
	tpmNonNotify   = 0x0080
	tpmReturnCmd   = 0x0100

	mbIconError = 0x00000010
)

// Menu command ids.
const (
	cmdOpen = iota + 1
	cmdOpenLocal
	cmdDataDir
	cmdLog
	cmdQuit
)

type point struct{ X, Y int32 }

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msgStruct struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// notifyIconData mirrors NOTIFYICONDATAW. The field order and padding must
// match the C struct exactly (976 bytes on x64).
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// Options configures the tray icon.
type Options struct {
	Title    string
	URL      string // the friendly address, e.g. http://kisaf.local
	AltURL   string // the always-works address, e.g. http://localhost:7777
	DataDir  string
	IconPath string
	OnQuit   func()
	Logf     func(string, ...any)
}

// state is package level because the window procedure is a C callback and
// cannot carry a Go closure.
var state struct {
	opts Options
	hwnd uintptr
	nid  notifyIconData
}

// Run shows the tray icon and pumps the Windows message loop until the user
// quits. It returns false if the icon could not be created, in which case the
// caller should fall back to blocking on signals.
func Run(opts Options, signals <-chan os.Signal) (ok bool) {
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	// A panic in win32 glue must never take the HTTP server down with it.
	defer func() {
		if r := recover(); r != nil {
			opts.Logf("tray icon crashed, continuing in the background: %v", r)
			ok = false
		}
	}()

	// The message loop must stay on the thread that created the window.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state.opts = opts

	hwnd, err := createMessageWindow()
	if err != nil {
		opts.Logf("could not create the tray window: %v", err)
		return false
	}
	state.hwnd = hwnd

	if err := addIcon(hwnd, opts); err != nil {
		opts.Logf("could not add the tray icon: %v", err)
		_, _, _ = procDestroyWindow.Call(hwnd)
		return false
	}

	// Ctrl+C and service stop requests arrive on a channel, but only the
	// message loop can shut the window down, so translate one into the other.
	go func() {
		<-signals
		_, _, _ = procPostMessage.Call(hwnd, wmClose, 0, 0)
	}()

	pumpMessages()
	return true
}

func createMessageWindow() (uintptr, error) {
	instance, _, _ := procGetModuleHandle.Call(0)
	className := mustUTF16("KisafTrayWindow")

	class := wndClassEx{
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     instance,
		LpszClassName: className,
	}
	class.CbSize = uint32(unsafe.Sizeof(class))

	if atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return 0, err
	}

	// A hidden window with no size: it exists only to receive tray callbacks.
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(mustUTF16("kisaf"))),
		0, 0, 0, 0, 0, 0, 0, instance, 0,
	)
	if hwnd == 0 {
		return 0, err
	}
	return hwnd, nil
}

func addIcon(hwnd uintptr, opts Options) error {
	nid := notifyIconData{
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            loadIcon(opts.IconPath),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	copyUTF16(nid.SzTip[:], opts.Title)

	state.nid = nid
	if ret, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&state.nid))); ret == 0 {
		return err
	}
	return nil
}

func loadIcon(path string) uintptr {
	if path != "" {
		h, _, _ := procLoadImage.Call(
			0,
			uintptr(unsafe.Pointer(mustUTF16(path))),
			imageIcon, 0, 0,
			lrLoadFromFile|lrDefaultSize,
		)
		if h != 0 {
			return h
		}
	}
	h, _, _ := procLoadIcon.Call(0, idiApplication)
	return h
}

func pumpMessages() {
	var m msgStruct
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		// 0 is WM_QUIT, -1 (all bits set) is an error; both end the loop.
		if ret == 0 || int32(ret) == -1 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// wndProc is called by Windows. Every parameter must be uintptr-sized for
// syscall.NewCallback to accept the function.
func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch message {
	case wmTrayCallback:
		switch lparam {
		case wmLButtonUp, wmLButtonDblClk:
			openBrowser(state.opts.URL, state.opts.AltURL)
		case wmRButtonUp, wmContextMenuKey:
			showMenu(hwnd)
		}
		return 0

	case wmClose:
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		removeIcon()
		if state.opts.OnQuit != nil {
			state.opts.OnQuit()
		}
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, message, wparam, lparam)
	return ret
}

func showMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendItem(menu, cmdOpen, "Open kisaf  ("+state.opts.URL+")")
	appendItem(menu, cmdOpenLocal, "Open via localhost  ("+state.opts.AltURL+")")
	appendSeparator(menu)
	appendItem(menu, cmdDataDir, "Open data folder")
	appendItem(menu, cmdLog, "Open log file")
	appendSeparator(menu)
	appendItem(menu, cmdQuit, "Quit")

	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// Without this the menu refuses to close when the user clicks elsewhere —
	// a documented quirk of tray menus since Windows 95.
	_, _, _ = procSetForegroundWindow.Call(hwnd)

	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCmd|tpmNonNotify,
		uintptr(pt.X), uintptr(pt.Y),
		0, hwnd, 0,
	)

	switch cmd {
	case cmdOpen:
		openBrowser(state.opts.URL, state.opts.AltURL)
	case cmdOpenLocal:
		openBrowser(state.opts.AltURL, "")
	case cmdDataDir:
		openPath(state.opts.DataDir)
	case cmdLog:
		openPath(filepath.Join(state.opts.DataDir, "kisaf.log"))
	case cmdQuit:
		_, _, _ = procPostMessage.Call(hwnd, wmClose, 0, 0)
	}
}

func appendItem(menu uintptr, id uintptr, label string) {
	_, _, _ = procAppendMenu.Call(menu, mfString, id, uintptr(unsafe.Pointer(mustUTF16(label))))
}

func appendSeparator(menu uintptr) {
	_, _, _ = procAppendMenu.Call(menu, mfSeparator, 0, 0)
}

func removeIcon() {
	_, _, _ = procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&state.nid)))
}

// openBrowser tries the pretty URL first and silently falls back to the
// loopback one, which cannot fail to resolve.
func openBrowser(primary, fallback string) {
	if err := shellOpen(primary); err != nil && fallback != "" {
		_ = shellOpen(fallback)
	}
}

func shellOpen(target string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func openPath(path string) {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}

// Alert shows a modal error box. On a windowsgui build there is no console, so
// this is the only way a startup failure can reach the user.
func Alert(title, message string) {
	_, _, _ = procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16(message))),
		uintptr(unsafe.Pointer(mustUTF16(title))),
		mbIconError,
	)
}

func mustUTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		// Only possible if s contains a NUL, which none of our literals do.
		p, _ = syscall.UTF16PtrFromString("kisaf")
	}
	return p
}

func copyUTF16(dst []uint16, s string) {
	encoded := syscall.StringToUTF16(s)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)]
		encoded[len(encoded)-1] = 0
	}
	copy(dst, encoded)
}
