//go:build windows

package nativefill

import (
	"context"
	"errors"
	"time"
)

// ErrVendorWindow means no visible vendor window matched the process hints.
var ErrVendorWindow = errors.New("vendor login window not found")

// ErrVendorLoginIncomplete means a window exists but login fields are not ready.
var ErrVendorLoginIncomplete = errors.New("vendor login fields not ready")

// ErrVendorClickUsername means fallback typing needs the username field focused.
var ErrVendorClickUsername = errors.New("click the username field in the vendor login window")

// FillVendorLoginWindow sets username and password in a vendor login window, or
// types them into the focused field when the client does not use standard edits.
func FillVendorLoginWindow(ctx context.Context, processHints []string, username, password string) error {
	windows := FindVendorWindows(processHints)
	if len(windows) == 0 {
		return ErrVendorWindow
	}

	for _, w := range windows {
		plain, secret := LoginFields(w.HWND)
		if len(plain) == 0 || len(secret) == 0 {
			continue
		}
		if username != "" {
			if err := SetFieldText(plain[0], username); err != nil {
				return err
			}
		}
		if password != "" {
			if err := SetFieldText(secret[0], password); err != nil {
				return err
			}
		}
		BringToFront(w.HWND)
		return nil
	}

	front := windows[0]
	if !IsForeground(front.HWND) {
		return ErrVendorClickUsername
	}
	WaitForModifierRelease(ctx)
	if username != "" {
		if err := TypeIntoFocused(username); err != nil {
			return err
		}
	}
	if password != "" {
		if username != "" {
			if err := PressTab(); err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err := TypeIntoFocused(password); err != nil {
			return err
		}
	}
	return nil
}

// ProbeVendorLoginWindow reports whether the user can press the fill shortcut.
func ProbeVendorLoginWindow(processHints []string) error {
	windows := FindVendorWindows(processHints)
	if len(windows) == 0 {
		return ErrVendorWindow
	}
	anyEdit := false
	for _, w := range windows {
		plain, secret := LoginFields(w.HWND)
		if len(plain) > 0 && len(secret) > 0 {
			return nil
		}
		anyEdit = anyEdit || len(plain) > 0 || len(secret) > 0
	}
	if !anyEdit {
		return nil
	}
	return ErrVendorLoginIncomplete
}
