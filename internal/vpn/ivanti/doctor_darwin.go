//go:build darwin

package ivanti

import (
	"context"
	"os"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/doctor"
)

const (
	fullSummary = "This customer takes over all traffic"
	fullDetail  = "Ivanti installed a full-tunnel route. Other programs will use this customer's network until you disconnect."

	splitSummary = "Split tunnel"
	splitDetail  = "Only this customer's networks go through the VPN. Other programs keep your usual internet."
)

// DiagnoseNetwork reports whether this Ivanti profile's live tunnel captures
// every packet, using the client's route-recovery file and the kernel table.
func (p *Provider) DiagnoseNetwork(ctx context.Context, profile domain.VPNProfile) (domain.NetworkDiagnosis, error) {
	reading, err := readTunnel()
	if err != nil || !reading.live || !reading.belongsTo(ctx, profile.Host) {
		return domain.NetworkDiagnosis{Known: false}, nil
	}

	captures := false
	if raw, readErr := os.ReadFile(routeRecoveryPath); readErr == nil {
		captures = recoveryCapturesAll(parseRecovery(string(raw)))
	}

	if table, tableErr := doctor.ReadInetTable(ctx); tableErr == nil {
		if doctor.CapturesAllTraffic(table, reading.iface, reading.assignedIP) {
			captures = true
		}
	}

	diag := domain.NetworkDiagnosis{
		Known:              true,
		CapturesAllTraffic: captures,
		Summary:            splitSummary,
		Detail:             splitDetail,
	}
	if captures {
		diag.Summary = fullSummary
		diag.Detail = fullDetail
	}
	return diag, nil
}
