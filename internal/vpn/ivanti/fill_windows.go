//go:build windows

package ivanti

import (
	"context"
	"errors"
	"fmt"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nativefill"
)

// FillNativeLogin fills the Ivanti/Pulse login window. Connect is never pressed.
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
	err := nativefill.FillVendorLoginWindow(ctx, ivantiProcessHints, credential.Username, credential.Secret)
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

// ProbeNativeLogin reports whether an Ivanti login window is ready for fill.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	err := nativefill.ProbeVendorLoginWindow(ivantiProcessHints)
	if err == nil {
		return nil
	}
	if errors.Is(err, nativefill.ErrVendorWindow) || errors.Is(err, nativefill.ErrVendorLoginIncomplete) {
		return errFillWaiting
	}
	return err
}
