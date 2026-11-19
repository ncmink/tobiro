//go:build !darwin

package wireguard

import (
	"context"
	"runtime"
	"time"

	"tobiro/internal/domain"
)

const unsupportedReason = "WireGuard is only supported on macOS in this release. " +
	"The wg-quick CLI from wireguard.com needs root and is not driven yet on this platform."

// Detect reports that WireGuard is not available on this platform.
func (p *Provider) Detect(ctx context.Context) (*domain.ClientInfo, error) {
	return &domain.ClientInfo{
		Provider:   domain.ProviderWireGuard,
		Platform:   runtime.GOOS,
		DetectedAt: time.Now().Format(time.RFC3339),
		Detail:     unsupportedReason,
	}, nil
}

// Capabilities reports no control off macOS.
func (p *Provider) Capabilities(ctx context.Context) (*domain.Capabilities, error) {
	info, err := p.Detect(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.Capabilities{Notes: info.Detail}, nil
}

// ListVendorProfiles is unavailable off macOS.
func (p *Provider) ListVendorProfiles(ctx context.Context) ([]domain.VendorProfileOption, error) {
	return nil, p.unsupported("list tunnels")
}

// Connect is unavailable off macOS.
func (p *Provider) Connect(ctx context.Context, _ domain.VPNProfile, _ domain.Credential) error {
	return p.unsupported("connect")
}

// Disconnect is unavailable off macOS.
func (p *Provider) Disconnect(ctx context.Context, _ domain.VPNProfile) error {
	return p.unsupported("disconnect")
}

// Status is unavailable off macOS.
func (p *Provider) Status(ctx context.Context, _ domain.VPNProfile) (*domain.VPNStatus, error) {
	return &domain.VPNStatus{
		Known:  false,
		Detail: unsupportedReason,
	}, nil
}

// OpenNativeClient is unavailable off macOS.
func (p *Provider) OpenNativeClient(ctx context.Context, _ domain.VPNProfile) error {
	return p.unsupported("open native client")
}

func (p *Provider) unsupported(operation string) error {
	return &domain.UnsupportedOperationError{
		Provider:  domain.ProviderWireGuard,
		Operation: operation,
		Platform:  runtime.GOOS,
		Reason:    unsupportedReason,
	}
}
