//go:build windows

package ivanti

import (
	"context"
	"errors"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/tunnel"
)

func listConnections() ([]domain.VendorProfileOption, error) {
	return nil, errors.New("saved Ivanti connections are only read on macOS")
}

// statusSupported reports that Windows status reading is implemented.
func statusSupported() bool { return true }

// readStatus reports the live tunnel from the machine's own network state: the
// Ivanti service's control socket to the gateway, or the Ivanti virtual
// adapter being up. Nothing is read from or sent to the Ivanti client itself.
func readStatus(ctx context.Context, profile domain.VPNProfile) *domain.VPNStatus {
	return tunnel.Status(tunnel.ProbeWith(ctx, tunnel.Options{
		Host: profile.Host,
		Port: profile.Port,
		// dsAccessService.exe holds the tunnel; Pulse.exe is only the window.
		ProcessHints: []string{"dsAccessService", "dsNcService", "Pulse", "Ivanti"},
		AdapterHints: []string{"Juniper Networks Virtual Adapter", "Pulse Secure", "Ivanti"},
	}))
}

// readSnapshot is unavailable: the saved-connection store and event log that
// tell cards on one gateway apart are only read on macOS.
func readSnapshot(context.Context, string, int) (snapshot, bool) {
	return snapshot{}, false
}
