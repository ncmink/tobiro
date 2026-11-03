//go:build !darwin

package ivanti

import (
	"context"
	"errors"

	"tobiro/internal/domain"
)

func listConnections() ([]domain.VendorProfileOption, error) {
	return nil, errors.New("saved Ivanti connections are only read on macOS")
}

// statusSupported reports that no status source has been verified for this
// platform yet.
func statusSupported() bool { return false }

// readStatus returns an unknown reading.
//
// Linux and Windows publish tunnel state differently (pulsesvc and
// pulselauncher respectively), and neither has been verified against a real
// installation, so nothing is claimed here.
func readStatus(_ context.Context, _ domain.VPNProfile) *domain.VPNStatus {
	return &domain.VPNStatus{
		Known:  false,
		Detail: "Tunnel status is not read from the Ivanti client on this platform; check the Ivanti Secure Access window.",
	}
}

func readSnapshot(context.Context, string, int) (snapshot, bool) {
	return snapshot{}, false
}
