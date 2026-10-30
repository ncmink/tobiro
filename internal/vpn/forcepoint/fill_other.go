//go:build !darwin

package forcepoint

import (
	"context"

	"tobiro/internal/domain"
)

// FillNativeLogin is a no-op off macOS: Forcepoint's login dialog is only
// driven through Accessibility on darwin.
func (p *Provider) FillNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return nil
}

// ProbeNativeLogin is a no-op off macOS.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return nil
}
