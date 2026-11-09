//go:build windows

package nativefill

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procIsWindowEnabled     = user32.NewProc("IsWindowEnabled")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procSendInput           = user32.NewProc("SendInput")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
)

const (
	gwlStyle        = ^uintptr(15) // GWL_STYLE (-16)
	esPassword      = 0x0020
	wmSetText       = 0x000C
	smtoAbortIfHung = 0x0002
	swRestore       = 9

	inputKeyboard     = 1
	keyEventFKeyUp    = 0x0002
	keyEventFUnicode  = 0x0004
	vkTab             = 0x09
	vkShift           = 0x10
	vkControl         = 0x11
	vkMenu            = 0x12
	vkLWin            = 0x5B
	vkRWin            = 0x5C
	vkA               = 0x41
	setTextTimeoutMs  = 2000
	modifierWaitLimit = 1500 * time.Millisecond
)

// VendorWindow is a visible, top-level window owned by a vendor process.
type VendorWindow struct {
	HWND    windows.HWND
	Process string
}

// The callbacks are created once: Windows never frees a NewCallback thunk.
var (
	enumMu       sync.Mutex
	enumFound    []windows.HWND
	enumTopLevel = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		if windows.IsWindowVisible(hwnd) {
			enumFound = append(enumFound, hwnd)
		}
		return 1
	})
	enumChild = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		enumFound = append(enumFound, hwnd)
		return 1
	})
)

func topLevelWindows() []windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = nil
	_ = windows.EnumWindows(enumTopLevel, nil)
	out := enumFound
	enumFound = nil
	return out
}

func childWindows(parent windows.HWND) []windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumFound = nil
	windows.EnumChildWindows(parent, enumChild, nil)
	out := enumFound
	enumFound = nil
	return out
}

// FindVendorWindows returns visible top-level windows whose process image
// name contains one of hints (case-insensitive). The foreground window, when it
// matches, comes first.
func FindVendorWindows(hints []string) []VendorWindow {
	front := windows.GetForegroundWindow()
	var first, rest []VendorWindow
	for _, hwnd := range topLevelWindows() {
		name := windowProcess(hwnd)
		if name == "" || !matchesAny(name, hints) {
			continue
		}
		w := VendorWindow{HWND: hwnd, Process: name}
		if hwnd == front {
			first = append(first, w)
		} else {
			rest = append(rest, w)
		}
	}
	return append(first, rest...)
}

// LoginFields returns the enabled, visible Win32 edit controls inside a window,
// split into plain text and password (ES_PASSWORD) fields in tab order.
func LoginFields(hwnd windows.HWND) (plain, secret []windows.HWND) {
	for _, child := range childWindows(hwnd) {
		if !windows.IsWindowVisible(child) || !isEnabled(child) {
			continue
		}
		if !strings.Contains(strings.ToLower(className(child)), "edit") {
			continue
		}
		style, _, _ := procGetWindowLongPtrW.Call(uintptr(child), gwlStyle)
		if style&esPassword != 0 {
			secret = append(secret, child)
		} else {
			plain = append(plain, child)
		}
	}
	return plain, secret
}

// SetFieldText replaces the text of an edit control in another process. The
// control raises EN_CHANGE itself, so the vendor dialog sees the new value.
func SetFieldText(field windows.HWND, text string) error {
	ptr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	var result uintptr
	ret, _, callErr := procSendMessageTimeoutW.Call(
		uintptr(field), wmSetText, 0, uintptr(unsafe.Pointer(ptr)),
		smtoAbortIfHung, setTextTimeoutMs, uintptr(unsafe.Pointer(&result)),
	)
	if ret == 0 {
		return callErr
	}
	return nil
}

// BringToFront restores and activates a window. Windows only allows this from
// the process that received the last input event, which the fill hotkey is.
func BringToFront(hwnd windows.HWND) {
	if iconic, _, _ := procIsIconic.Call(uintptr(hwnd)); iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore)
	}
	procSetForegroundWindow.Call(uintptr(hwnd))
}

// IsForeground reports whether hwnd is the active window.
func IsForeground(hwnd windows.HWND) bool {
	return windows.GetForegroundWindow() == hwnd
}

// WaitForModifierRelease waits until the user lets go of Ctrl, Shift, Alt and
// Win after pressing the fill shortcut, so synthesized characters are not read
// as shortcuts by the vendor window.
func WaitForModifierRelease(ctx context.Context) {
	deadline := time.Now().Add(modifierWaitLimit)
	for time.Now().Before(deadline) {
		held := false
		for _, vk := range []uintptr{vkControl, vkShift, vkMenu, vkLWin, vkRWin} {
			if state, _, _ := procGetAsyncKeyState.Call(vk); state&0x8000 != 0 {
				held = true
				break
			}
		}
		if !held {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Millisecond):
		}
	}
}

// TypeIntoFocused selects the focused field's contents and types text as
// Unicode keystrokes. Enter is never sent.
func TypeIntoFocused(text string) error {
	if err := sendKeys(keyDown(vkControl), keyDown(vkA), keyUp(vkA), keyUp(vkControl)); err != nil {
		return err
	}
	var events []keyboardInput
	for _, unit := range windows.StringToUTF16(text) {
		if unit == 0 {
			break
		}
		events = append(events,
			keyboardInput{Type: inputKeyboard, Scan: unit, Flags: keyEventFUnicode},
			keyboardInput{Type: inputKeyboard, Scan: unit, Flags: keyEventFUnicode | keyEventFKeyUp},
		)
	}
	return sendKeys(events...)
}

// PressTab moves focus to the next field.
func PressTab() error {
	return sendKeys(keyDown(vkTab), keyUp(vkTab))
}

// keyboardInput mirrors INPUT with a KEYBDINPUT payload on 64-bit Windows,
// padded to sizeof(INPUT) = 40.
type keyboardInput struct {
	Type      uint32
	_         uint32
	VK        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	_         uint32
	ExtraInfo uintptr
	_         [8]byte
}

func keyDown(vk uint16) keyboardInput {
	return keyboardInput{Type: inputKeyboard, VK: vk}
}

func keyUp(vk uint16) keyboardInput {
	return keyboardInput{Type: inputKeyboard, VK: vk, Flags: keyEventFKeyUp}
}

func sendKeys(events ...keyboardInput) error {
	if len(events) == 0 {
		return nil
	}
	sent, _, err := procSendInput.Call(
		uintptr(len(events)),
		uintptr(unsafe.Pointer(&events[0])),
		unsafe.Sizeof(events[0]),
	)
	if int(sent) != len(events) {
		return err
	}
	return nil
}

func isEnabled(hwnd windows.HWND) bool {
	ret, _, _ := procIsWindowEnabled.Call(uintptr(hwnd))
	return ret != 0
}

func className(hwnd windows.HWND) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(hwnd, &buf[0], int32(len(buf)))
	if err != nil || n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// windowProcess returns the image name of the process owning hwnd, without
// the .exe suffix (e.g. "sgvpn").
func windowProcess(hwnd windows.HWND) string {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return ""
	}
	return processImageName(pid)
}

func processImageName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	base := filepath.Base(windows.UTF16ToString(buf[:size]))
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func matchesAny(name string, hints []string) bool {
	lower := strings.ToLower(name)
	for _, hint := range hints {
		if hint = strings.ToLower(strings.TrimSpace(hint)); hint != "" && strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}
