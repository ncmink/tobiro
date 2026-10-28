//go:build !darwin

package surfshark

import (
	"context"

	"tobiro/internal/domain"
)

// statusSupported reports that the tunnel state is only read on macOS.
func statusSupported() bool { return false }

func readStatus(context.Context) *domain.VPNStatus {
	return &domain.VPNStatus{
		Known:  false,
		Detail: "Tunnel status is not read on this platform; check the Surfshark window.",
	}
}

func tunnelProviderDetail(context.Context) string {
	return "Its tunnel is started and stopped in the Surfshark window."
}
