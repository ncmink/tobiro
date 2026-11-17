//go:build darwin

package openvpn

import (
	"context"

	"tobiro/internal/domain"
)

func statusSupported() bool { return true }

func readStatus(ctx context.Context, p *Provider, profile domain.VPNProfile) *domain.VPNStatus {
	p.detectPaths(ctx)
	if p.cliPath == "" {
		return &domain.VPNStatus{Known: false}
	}
	if st, ok := statusFromLog(ctx, p.cliPath, platformLogPath(), profile); ok {
		return st
	}
	return &domain.VPNStatus{Known: false}
}