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
// Ivanti virtual adapter must be up and a socket to this gateway must exist.
// Nothing is read from or sent to the Ivanti client itself.
func readStatus(ctx context.Context, profile domain.VPNProfile) *domain.VPNStatus {
	return readGatewayStatus(ctx, profile.Host, profile.Port)
}

func readGatewayStatus(ctx context.Context, host string, port int) *domain.VPNStatus {
	return tunnel.Status(tunnel.ProbeWith(ctx, tunnel.Options{
		Host: host,
		Port: port,
		// dsAccessService.exe holds the tunnel; Pulse.exe is only the window.
		// PulseSecureService.exe talks to the gateway while idle, which is
		// why the adapter is required.
		ProcessHints:   []string{"dsAccessService", "dsNcService", "Pulse", "Ivanti"},
		AdapterHints:   []string{"Juniper Networks Virtual Adapter", "Pulse Secure", "Ivanti"},
		RequireAdapter: true,
	}))
}

// readSnapshot reads one gateway's tunnel. The saved-connection store and event
// log that name the active connection are only read on macOS, so cards sharing
// a gateway are told apart by Tobiro's own sessions instead.
func readSnapshot(ctx context.Context, host string, port int) (snapshot, bool) {
	return snapshot{gatewayStatus: *readGatewayStatus(ctx, host, port)}, true
}
