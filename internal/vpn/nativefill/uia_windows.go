//go:build windows

package nativefill

import (
	"context"
	"errors"
	"runtime"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// UI Automation reaches login fields that are not Win32 edit controls, such as
// the HTML inputs of Electron clients (FortiClient). Fields are only focused
// through UIA; the text itself is still typed as keystrokes, because web forms
// ignore values that are set without input events.

var (
	ole32                = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidCUIAutomation = windows.GUID{Data1: 0xff48dba4, Data2: 0x60ef, Data3: 0x4201, Data4: [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iidIUIAutomation   = windows.GUID{Data1: 0x30cbe57d, Data2: 0xd9d0, Data3: 0x452a, Data4: [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
)

const (
	treeScopeDescendants = 4
	uiaEditControlTypeID = 50004

	// IUIAutomation vtable slots.
	uiaElementFromHandle   = 6
	uiaGetFocusedElement   = 8
	uiaCreateTrueCondition = 21

	// IUIAutomationElement vtable slots.
	elemSetFocus           = 3
	elemFindAll            = 6
	elemControlType        = 21
	elemIsEnabled          = 28
	elemIsPassword         = 35
	elemIsOffscreen        = 38
	elemBoundingRectangle  = 43
	arrLength              = 3
	arrGetElement          = 4
	comRelease             = 2
	chromiumTreeRetries    = 8
	chromiumTreeRetryDelay = 250 * time.Millisecond
)

var errUIAUnavailable = errors.New("UI Automation is unavailable")

// comIface is a COM interface pointer; its first word is the vtable.
type comIface struct {
	vtbl *[64]uintptr
}

func (c *comIface) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(c))}, args...)
	hr, _, _ := syscall.SyscallN(c.vtbl[method], all...)
	return hr
}

func (c *comIface) release() {
	if c != nil {
		c.call(comRelease)
	}
}

// withUIA runs fn on a locked OS thread with COM and a UIA client ready.
func withUIA(fn func(uia *comIface) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil &&
		!errors.Is(err, windows.Errno(windows.RPC_E_CHANGED_MODE)) {
		return errUIAUnavailable
	} else if err == nil {
		defer windows.CoUninitialize()
	}

	var uia *comIface
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, windows.CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&uia)),
	)
	if hr != 0 || uia == nil {
		return errUIAUnavailable
	}
	defer uia.release()
	return fn(uia)
}

type uiaField struct {
	elem     *comIface
	top      int32
	password bool
}

// uiaLoginFields lists enabled, on-screen edit elements under hwnd, top to
// bottom. Chromium builds its accessibility tree only after a client first
// asks for it, so an empty result is retried briefly.
func uiaLoginFields(ctx context.Context, uia *comIface, hwnd windows.HWND) []uiaField {
	var root *comIface
	if uia.call(uiaElementFromHandle, uintptr(hwnd), uintptr(unsafe.Pointer(&root))) != 0 || root == nil {
		return nil
	}
	defer root.release()
	var cond *comIface
	if uia.call(uiaCreateTrueCondition, uintptr(unsafe.Pointer(&cond))) != 0 || cond == nil {
		return nil
	}
	defer cond.release()

	for attempt := 0; attempt < chromiumTreeRetries; attempt++ {
		if fields := findEdits(root, cond); len(fields) > 0 {
			return fields
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(chromiumTreeRetryDelay):
		}
	}
	return nil
}

func findEdits(root, cond *comIface) []uiaField {
	var arr *comIface
	if root.call(elemFindAll, treeScopeDescendants, uintptr(unsafe.Pointer(cond)), uintptr(unsafe.Pointer(&arr))) != 0 || arr == nil {
		return nil
	}
	defer arr.release()
	var n int32
	arr.call(arrLength, uintptr(unsafe.Pointer(&n)))

	var fields []uiaField
	for i := int32(0); i < n; i++ {
		var el *comIface
		if arr.call(arrGetElement, uintptr(i), uintptr(unsafe.Pointer(&el))) != 0 || el == nil {
			continue
		}
		if !isUsableEdit(el) {
			el.release()
			continue
		}
		var rect windows.Rect
		el.call(elemBoundingRectangle, uintptr(unsafe.Pointer(&rect)))
		fields = append(fields, uiaField{elem: el, top: rect.Top, password: boolProp(el, elemIsPassword)})
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].top < fields[j].top })
	return fields
}

func isUsableEdit(el *comIface) bool {
	var controlType int32
	if el.call(elemControlType, uintptr(unsafe.Pointer(&controlType))) != 0 || controlType != uiaEditControlTypeID {
		return false
	}
	return boolProp(el, elemIsEnabled) && !boolProp(el, elemIsOffscreen)
}

func boolProp(el *comIface, slot int) bool {
	var v int32
	if el.call(slot, uintptr(unsafe.Pointer(&v))) != 0 {
		return false
	}
	return v != 0
}

func releaseFields(fields []uiaField) {
	for _, f := range fields {
		f.elem.release()
	}
}

// fillWithUIA focuses the username and password elements of hwnd in turn and
// types into each. It reports false when the window exposes no usable pair.
func fillWithUIA(ctx context.Context, hwnd windows.HWND, username, password string) (bool, error) {
	filled := false
	err := withUIA(func(uia *comIface) error {
		fields := uiaLoginFields(ctx, uia, hwnd)
		defer releaseFields(fields)

		var user, pass *comIface
		for _, f := range fields {
			switch {
			case f.password && pass == nil:
				pass = f.elem
			case !f.password && user == nil && pass == nil:
				// The username field sits above the password field.
				user = f.elem
			}
		}
		if pass == nil || (username != "" && user == nil) {
			return nil
		}

		BringToFront(hwnd)
		time.Sleep(100 * time.Millisecond)
		if !IsForeground(hwnd) {
			return ErrVendorClickUsername
		}
		WaitForModifierRelease(ctx)

		if username != "" {
			if err := focusAndType(uia, user, username); err != nil {
				return err
			}
		}
		if password != "" {
			if err := focusAndType(uia, pass, password); err != nil {
				return err
			}
		}
		filled = true
		return nil
	})
	if errors.Is(err, errUIAUnavailable) {
		return false, nil
	}
	return filled, err
}

// focusAndType focuses el and, only once focus is confirmed to be in an edit
// field, replaces its contents with text.
func focusAndType(uia, el *comIface, text string) error {
	if el.call(elemSetFocus) != 0 {
		return ErrVendorClickUsername
	}
	time.Sleep(80 * time.Millisecond)
	if !focusedIsEdit(uia) {
		return ErrVendorClickUsername
	}
	return TypeIntoFocused(text)
}

func focusedIsEdit(uia *comIface) bool {
	var el *comIface
	if uia.call(uiaGetFocusedElement, uintptr(unsafe.Pointer(&el))) != 0 || el == nil {
		return false
	}
	defer el.release()
	var controlType int32
	return el.call(elemControlType, uintptr(unsafe.Pointer(&controlType))) == 0 && controlType == uiaEditControlTypeID
}

// FocusIsInEditField reports whether keyboard focus is in a text field, so
// keystrokes (including Ctrl+A) land in it rather than selecting a whole page.
func FocusIsInEditField() bool {
	ok := false
	_ = withUIA(func(uia *comIface) error {
		ok = focusedIsEdit(uia)
		return nil
	})
	return ok
}
