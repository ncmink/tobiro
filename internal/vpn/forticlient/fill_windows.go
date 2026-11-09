//go:build windows

package forticlient

import (
	"context"
	"errors"
	"fmt"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nativefill"
)

// FillNativeLogin fills FortiClient's login window. Connect is never pressed.
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
	err := nativefill.FillVendorLoginWindow(ctx, forticlientProcessHints, credential.Username, credential.Secret)
	if err == nil {
		return nil
	}
	if errors.Is(err, nativefill.ErrVendorClickUsername) {
		return errors.New("click the username field in the FortiClient login window, then press the fill shortcut")
	}
	if errors.Is(err, nativefill.ErrVendorWindow) || errors.Is(err, nativefill.ErrVendorLoginIncomplete) {
		return fmt.Errorf("%w: %v", errFillWaiting, err)
	}
	return err
}

// ProbeNativeLogin reports whether FortiClient's login window is ready for fill.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	err := nativefill.ProbeVendorLoginWindow(forticlientProcessHints)
	if err == nil {
		return nil
	}
	if errors.Is(err, nativefill.ErrVendorWindow) || errors.Is(err, nativefill.ErrVendorLoginIncomplete) {
		return errFillWaiting
	}
	return err
}
