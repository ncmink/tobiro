//go:build !darwin && !windows

package openvpn

import (
	"context"

	"tobiro/internal/domain"
)

func statusSupported() bool { return false }

func readStatus(context.Context, *Provider, domain.VPNProfile) *domain.VPNStatus {
	return &domain.VPNStatus{Known: false}
}