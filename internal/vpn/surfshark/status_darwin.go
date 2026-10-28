//go:build darwin

package surfshark

import (
	"context"
	"strings"

	"tobiro/internal/domain"
	"tobiro/internal/sysexec"
	"tobiro/internal/vpn/nc"
)

const sysextPath = "/usr/bin/systemextensionsctl"

// statusSupported reports whether the tunnel state can be read on this platform.
func statusSupported() bool { return nc.Supported() }

// readStatus reports whether Surfshark's own VPN service is connected.
//
// Surfshark's macOS client runs its tunnel as a NetworkExtension, so macOS —
// not Surfshark — owns the session state, and `scutil --nc` reports it per
// service together with the owning app's bundle identifier. That identifier is
// what attributes a reading to Surfshark.
//
// Inferring the state from the machine's network interfaces instead does not
// work here. Surfshark has no gateway to hold a control socket against, and its
// tunnel extension keeps running after a disconnect, so "the extension is alive
// and something owns the default route" reports another vendor's tunnel — an
// L2TP or Tailscale session — as if it were Surfshark's.
func readStatus(ctx context.Context) *domain.VPNStatus {
	services, err := nc.List(ctx)
	if err != nil {
		return &domain.VPNStatus{Known: false, Detail: err.Error()}
	}

	surfshark := ownedBySurfshark(services)
	if len(surfshark) == 0 {
		return &domain.VPNStatus{
			Known:  false,
			Detail: "macOS lists no Surfshark VPN configuration yet. Connect once in the Surfshark app so it registers one.",
		}
	}

	connected, ok := firstConnected(surfshark)
	if !ok {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: "Surfshark's tunnel is not connected.",
		}
	}

	status := &domain.VPNStatus{
		State:  domain.StateConnected,
		Known:  true,
		Detail: "Surfshark reports " + connected.Name + " as connected.",
	}

	// The address is only reported when macOS publishes one for this service.
	// Picking a tunnel interface off the machine instead is what previously
	// attributed another vendor's tunnel to Surfshark.
	if reading, err := nc.Status(ctx, connected.ID); err == nil {
		status.AssignedIP = reading.AssignedIP
		status.Interface = reading.Interface
		if reading.Interface != "" && reading.AssignedIP != "" {
			status.Detail = "Tunnel up on " + reading.Interface + " with address " + reading.AssignedIP + "."
		}
	}
	return status
}

// ownedBySurfshark keeps the services registered by Surfshark's app.
//
// Matching the vendor name rather than a fixed bundle identifier covers both the
// direct-download and App Store builds, and an upgrade can leave more than one
// registered configuration behind.
func ownedBySurfshark(services []nc.Service) []nc.Service {
	var out []nc.Service
	for _, service := range services {
		if strings.Contains(strings.ToLower(service.ProviderID()), "surfshark") {
			out = append(out, service)
		}
	}
	return out
}

func firstConnected(services []nc.Service) (nc.Service, bool) {
	for _, service := range services {
		if service.Connected {
			return service, true
		}
	}
	return nc.Service{}, false
}

// extensionApproval is whether macOS will let Surfshark's tunnel run.
type extensionApproval int

const (
	extensionUnknown extensionApproval = iota
	extensionEnabled
	extensionNotApproved
)

// tunnelProviderDetail explains the state of Surfshark's tunnel extension.
//
// An unapproved system extension is the usual reason an otherwise healthy
// Surfshark install cannot connect at all, so it is worth surfacing where the
// user can act on it.
func tunnelProviderDetail(ctx context.Context) string {
	const fallback = "Its tunnel is started and stopped in the Surfshark window."

	res, err := sysexec.Run(ctx, sysextPath, "list")
	if err != nil && res.Stdout == "" {
		return fallback
	}

	switch extensionState(res.Stdout) {
	case extensionEnabled:
		return "Its tunnel extension is approved, and the tunnel is started and stopped in the Surfshark window."
	case extensionNotApproved:
		return "Its tunnel extension is not approved yet — allow it under System Settings > General > Login Items & Extensions > Network Extensions."
	default:
		return fallback
	}
}

// extensionState reads `systemextensionsctl list` for Surfshark's tunnel.
func extensionState(raw string) extensionApproval {
	found := false
	for _, line := range strings.Split(raw, "\n") {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "surfshark") || !strings.Contains(lower, "packettunnel") {
			continue
		}
		if strings.Contains(lower, "dnsproxy") {
			continue
		}
		found = true
		if strings.Contains(lower, "[activated enabled]") {
			return extensionEnabled
		}
	}
	if found {
		return extensionNotApproved
	}
	return extensionUnknown
}
