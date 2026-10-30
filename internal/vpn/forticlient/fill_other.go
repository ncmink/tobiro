//go:build !darwin

package forticlient

import (
	"context"

	"tobiro/internal/domain"
)

// FillNativeLogin is a no-op off macOS. FortiClient's password field is only
// typed through Accessibility on darwin.
func (p *Provider) FillNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return nil
}

// ProbeNativeLogin is a no-op off macOS.
func (p *Provider) ProbeNativeLogin(_ context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return nil
}
