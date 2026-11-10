//go:build windows

package nativefill

import (
	"context"
	"errors"
	"fmt"
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
//
// Failures that mean "nothing usable was found" are wrapped with a description
// of the foreground window, so the log shows what the vendor UI really is.
func FillVendorLoginWindow(ctx context.Context, processHints []string, username, password string) error {
	err := fillVendorLoginWindow(ctx, processHints, username, password)
	if errors.Is(err, ErrVendorWindow) || errors.Is(err, ErrVendorClickUsername) {
		return fmt.Errorf("%w (front window: %s)", err, DescribeForeground())
	}
	return err
}

func fillVendorLoginWindow(ctx context.Context, processHints []string, username, password string) error {
	windows := FindVendorWindows(processHints)
	if len(windows) == 0 {
		return ErrVendorWindow
	}

	for _, w := range windows {
		plain, secret := LoginFields(w.HWND)
		if len(plain) == 0 || len(secret) == 0 {
			continue
		}
		if BlockedByElevation(w.HWND) {
			return ErrVendorElevated
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

	// Web-based clients (Electron) expose their inputs through UI Automation.
	for _, w := range windows {
		if w.TitleOnly {
			continue
		}
		if BlockedByElevation(w.HWND) {
			return ErrVendorElevated
		}
		filled, err := fillWithUIA(ctx, w.HWND, username, password)
		if err != nil {
			return err
		}
		if filled {
			return nil
		}
	}

	// Last resort: type into the field the user clicked, but only in a window
	// the vendor process itself owns, that is in front, and whose focus is in a
	// text field. Otherwise Ctrl+A would select the whole page.
	var front *VendorWindow
	for i := range windows {
		if !windows[i].TitleOnly && IsForeground(windows[i].HWND) {
			front = &windows[i]
			break
		}
	}
	if front == nil {
		return ErrVendorClickUsername
	}
	if BlockedByElevation(front.HWND) {
		return ErrVendorElevated
	}
	WaitForModifierRelease(ctx)
	if !FocusIsInEditField() {
		return ErrVendorClickUsername
	}
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
			if !FocusIsInEditField() {
				return ErrVendorClickPassword
			}
		}
		if err := TypeIntoFocused(password); err != nil {
			return err
		}
	}
	return nil
}

// ErrVendorClickPassword means the username was typed but Tab did not land in
// the password field.
var ErrVendorClickPassword = errors.New("username filled; click the password field and press the fill shortcut again")

// ProbeVendorLoginWindow reports whether the user can press the fill shortcut.
func ProbeVendorLoginWindow(processHints []string) error {
	windows := FindVendorWindows(processHints)
	if len(windows) == 0 {
		return ErrVendorWindow
	}
	anyEdit := false
	anyOwned := false
	for _, w := range windows {
		plain, secret := LoginFields(w.HWND)
		if len(plain) > 0 && len(secret) > 0 {
			return nil
		}
		anyEdit = anyEdit || len(plain) > 0 || len(secret) > 0
		anyOwned = anyOwned || !w.TitleOnly
	}
	if !anyEdit && anyOwned {
		return nil
	}
	return ErrVendorLoginIncomplete
}
