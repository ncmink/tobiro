//go:build windows

package nativefill

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
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
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
)

const (
	gwlStyle          = ^uintptr(15) // GWL_STYLE (-16)
	esPassword        = 0x0020
	wmSetText         = 0x000C
	emGetPasswordChar = 0x00D2
	smtoAbortIfHung   = 0x0002
	swRestore         = 9

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
	// TitleOnly is set when only the window title matched. Such a window may
	// be a browser tab about the vendor, so keystrokes are never typed into it.
	TitleOnly bool
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

// FindVendorWindows returns visible top-level windows whose process image name
// or window title contains one of hints (case-insensitive). The title matters
// because a vendor's login dialog is often drawn by a helper process whose
// image name is not the product name. The foreground window, when it matches,
// comes first.
func FindVendorWindows(hints []string) []VendorWindow {
	front := windows.GetForegroundWindow()
	var first, rest []VendorWindow
	for _, hwnd := range topLevelWindows() {
		name := windowProcess(hwnd)
		byProcess := matchesAny(name, hints)
		if !byProcess && !matchesAny(windowTitle(hwnd), hints) {
			continue
		}
		w := VendorWindow{HWND: hwnd, Process: name, TitleOnly: !byProcess}
		if hwnd == front {
			first = append(first, w)
		} else {
			rest = append(rest, w)
		}
	}
	return append(first, rest...)
}

// LoginFields returns the enabled, visible Win32 edit controls inside a window,
// split into plain text and password fields, each ordered top to bottom.
//
// A field is a password field when the edit control says so (ES_PASSWORD or a
// password character). WinForms dialogs such as Forcepoint's "User
// Authentication" report neither, so those fields are classified by the label
// beside them ("User Name:", "Password:"), and as a last resort a pair of
// fields is read as username above password.
func LoginFields(hwnd windows.HWND) (plain, secret []windows.HWND) {
	var edits []control
	var labels []control
	for _, child := range childWindows(hwnd) {
		if !windows.IsWindowVisible(child) {
			continue
		}
		class := strings.ToLower(className(child))
		switch {
		case strings.Contains(class, "edit"):
			if isEnabled(child) {
				edits = append(edits, control{hwnd: child, rect: windowRect(child)})
			}
		case strings.Contains(class, "static"):
			if text := strings.TrimSpace(windowTitle(child)); text != "" {
				labels = append(labels, control{hwnd: child, rect: windowRect(child), text: text})
			}
		}
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].rect.Top < edits[j].rect.Top })

	var unknown []control
	for _, e := range edits {
		switch {
		case isPasswordEdit(e.hwnd):
			secret = append(secret, e.hwnd)
		default:
			switch labelKind(e, labels) {
			case kindPassword:
				secret = append(secret, e.hwnd)
			case kindUsername:
				plain = append(plain, e.hwnd)
			default:
				unknown = append(unknown, e)
			}
		}
	}

	// Only read a bare pair as username-over-password in a window that labels
	// a password somewhere, so a settings window's two text boxes are not
	// mistaken for a login form.
	if len(secret) == 0 && len(plain) == 0 && len(unknown) == 2 && hasPasswordLabel(labels) {
		return []windows.HWND{unknown[0].hwnd}, []windows.HWND{unknown[1].hwnd}
	}
	for _, e := range unknown {
		plain = append(plain, e.hwnd)
	}
	return plain, secret
}

type control struct {
	hwnd windows.HWND
	rect windows.Rect
	text string
}

type fieldKind int

const (
	kindUnknown fieldKind = iota
	kindUsername
	kindPassword
)

var (
	passwordLabels = []string{"password", "passcode", "รหัสผ่าน"}
	usernameLabels = []string{"user", "login", "account", "e-mail", "email", "ชื่อผู้ใช้"}
)

// labelKind classifies an edit by the nearest label on its row to the left, or
// directly above it.
func labelKind(edit control, labels []control) fieldKind {
	best := -1
	bestDist := int32(1 << 30)
	editMid := (edit.rect.Top + edit.rect.Bottom) / 2
	for i, l := range labels {
		var dist int32
		switch {
		case l.rect.Right <= edit.rect.Left+8 && l.rect.Top <= editMid && l.rect.Bottom >= editMid:
			dist = edit.rect.Left - l.rect.Right
		case l.rect.Bottom <= edit.rect.Top+4 && l.rect.Left < edit.rect.Right && l.rect.Right > edit.rect.Left:
			dist = edit.rect.Top - l.rect.Bottom + 1000
		default:
			continue
		}
		if dist < bestDist {
			best, bestDist = i, dist
		}
	}
	if best < 0 {
		return kindUnknown
	}
	text := strings.ToLower(labels[best].text)
	switch {
	case containsAny(text, passwordLabels):
		return kindPassword
	case containsAny(text, usernameLabels):
		return kindUsername
	}
	return kindUnknown
}

func hasPasswordLabel(labels []control) bool {
	for _, l := range labels {
		if containsAny(strings.ToLower(l.text), passwordLabels) {
			return true
		}
	}
	return false
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func isPasswordEdit(hwnd windows.HWND) bool {
	style, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), gwlStyle)
	if style&esPassword != 0 {
		return true
	}
	var char uintptr
	ret, _, _ := procSendMessageTimeoutW.Call(
		uintptr(hwnd), emGetPasswordChar, 0, 0,
		smtoAbortIfHung, 500, uintptr(unsafe.Pointer(&char)),
	)
	return ret != 0 && char != 0
}

func windowRect(hwnd windows.HWND) windows.Rect {
	var r windows.Rect
	procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r)))
	return r
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
		if errors.Is(callErr, windows.ERROR_ACCESS_DENIED) {
			return ErrVendorElevated
		}
		return callErr
	}
	return nil
}

// ErrVendorElevated means the vendor window belongs to a process running as
// administrator. Windows (UIPI) then drops text and keystrokes sent by a
// non-elevated app, without reporting an error for keystrokes.
var ErrVendorElevated = errors.New("the VPN client is running as administrator, so Windows blocks Tobiro from typing into it — run Tobiro as administrator too, or start the VPN client without administrator rights")

// BlockedByElevation reports whether hwnd's process is elevated while Tobiro
// is not.
func BlockedByElevation(hwnd windows.HWND) bool {
	if windows.GetCurrentProcessToken().IsElevated() {
		return false
	}
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil || pid == 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		// A medium-integrity process cannot open an elevated process's token.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer token.Close()
	return token.IsElevated()
}

// DescribeForeground summarizes the active window for diagnostics, e.g.
// `sgvpn "Forcepoint VPN Client" class=Qt5QWindowIcon edits=0/0`.
func DescribeForeground() string {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return "none"
	}
	plain, secret := LoginFields(hwnd)
	return fmt.Sprintf("%s %q class=%s edits=%d/%d",
		windowProcess(hwnd), windowTitle(hwnd), className(hwnd), len(plain), len(secret))
}

func windowTitle(hwnd windows.HWND) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
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
