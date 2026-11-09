//go:build windows

package forcepoint

import (
	"context"
	"errors"
	"fmt"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nativefill"
)

// FillNativeLogin puts the stored username and password into Forcepoint's
// login window. Connect is never pressed.
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
	err := nativefill.FillVendorLoginWindow(ctx, forcepointProcessHints, credential.Username, credential.Secret)
	if err == nil {
		return nil
	}
	if errors.Is(err, nativefill.ErrVendorClickUsername) {
		return fmt.Errorf("%w — then press the fill shortcut", err)
	}
	if errors.Is(err, nativefill.ErrVendorWindow) || errors.Is(err, nativefill.ErrVendorLoginIncomplete) {
		return errFillWaiting
	}
	return err
}

// ProbeNativeLogin reports whether a Forcepoint login window is open.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	err := nativefill.ProbeVendorLoginWindow(forcepointProcessHints)
	if err == nil {
		return nil
	}
	if errors.Is(err, nativefill.ErrVendorWindow) || errors.Is(err, nativefill.ErrVendorLoginIncomplete) {
		return errFillWaiting
	}
	return err
}
