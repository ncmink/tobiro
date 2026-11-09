//go:build windows

package forcepoint

import (
	"context"
	"errors"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nativefill"
)

// errFillNoFields means a Forcepoint window is open but exposes no standard
// edit controls, and it is not in front to receive keystrokes.
var errFillNoFields = errors.New("click the username field in the Forcepoint login window, then press the fill shortcut")

// FillNativeLogin puts the stored username and password into Forcepoint's
// login window. Standard Win32 edit controls are set directly, so it does not
// matter which field has focus; otherwise the account is typed into the
// focused field, Tab, then the password. Authenticate is never pressed.
func (p *Provider) FillNativeLogin(ctx context.Context, _ domain.VPNProfile, credential domain.Credential) error {
	if !canFill(credential) {
		return nil
	}
	if err := nativefill.ValidateFillValue("username", credential.Username); err != nil {
		return err
	}
	if err := nativefill.ValidateFillValue("password", credential.Secret); err != nil {
		return err
	}

	windows := nativefill.FindVendorWindows(forcepointProcessHints)
	if len(windows) == 0 {
		return errFillWaiting
	}

	for _, w := range windows {
		plain, secret := nativefill.LoginFields(w.HWND)
		if len(plain) == 0 || len(secret) == 0 {
			continue
		}
		if credential.Username != "" {
			if err := nativefill.SetFieldText(plain[0], credential.Username); err != nil {
				return err
			}
		}
		if credential.Secret != "" {
			if err := nativefill.SetFieldText(secret[0], credential.Secret); err != nil {
				return err
			}
		}
		nativefill.BringToFront(w.HWND)
		return nil
	}

	// No standard controls: type into whatever field the user clicked.
	front := windows[0]
	if !nativefill.IsForeground(front.HWND) {
		return errFillNoFields
	}
	nativefill.WaitForModifierRelease(ctx)
	if credential.Username != "" {
		if err := nativefill.TypeIntoFocused(credential.Username); err != nil {
			return err
		}
	}
	if credential.Secret != "" {
		if credential.Username != "" {
			if err := nativefill.PressTab(); err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err := nativefill.TypeIntoFocused(credential.Secret); err != nil {
			return err
		}
	}
	return nil
}

// ProbeNativeLogin reports whether a Forcepoint login window is open. A nil
// error means the user can press the fill shortcut.
//
// A window with no standard edit controls at all is also reported ready: the
// client draws its own fields, so the login form cannot be told apart from the
// main window.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	windows := nativefill.FindVendorWindows(forcepointProcessHints)
	if len(windows) == 0 {
		return errFillWaiting
	}
	anyEdit := false
	for _, w := range windows {
		plain, secret := nativefill.LoginFields(w.HWND)
		if len(plain) > 0 && len(secret) > 0 {
			return nil
		}
		anyEdit = anyEdit || len(plain) > 0 || len(secret) > 0
	}
	if !anyEdit {
		return nil
	}
	return errFillWaiting
}
