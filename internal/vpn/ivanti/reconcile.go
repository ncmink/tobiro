package ivanti

import (
	"context"
	"fmt"
	"strings"

	"tobiro/internal/domain"
)

// SessionReader supplies Tobiro session state for picking one connected card.
type SessionReader interface {
	Get(profileID string) (domain.VPNSession, bool)
}

// ReconcileProfiles reads the Ivanti tunnel once per gateway group and maps it
// to each connection card. Cards on the same gateway are distinguished by the
// saved Ivanti connection name (vendor profile), or by a unique name match.
func ReconcileProfiles(
	ctx context.Context,
	profiles []domain.VPNProfile,
	sessions SessionReader,
	apply func(domain.VPNProfile, domain.VPNStatus),
) {
	if len(profiles) == 0 || apply == nil {
		return
	}
	_ = sessions
	groups := groupByGateway(profiles)
	for key, group := range groups {
		host, port := splitGatewayKey(key)
		snap, ok := readSnapshot(ctx, host, port)
		if !ok {
			for _, p := range group {
				st := readStatus(ctx, p)
				if st != nil {
					apply(p, *st)
				}
			}
			continue
		}
		for _, p := range group {
			apply(p, statusForProfile(p, snap, len(group)))
		}
	}
}

func groupByGateway(profiles []domain.VPNProfile) map[string][]domain.VPNProfile {
	out := make(map[string][]domain.VPNProfile)
	for _, p := range profiles {
		key := gatewayKey(p.Host, p.Port)
		out[key] = append(out[key], p)
	}
	return out
}

func gatewayKey(host string, port int) string {
	return fmt.Sprintf("%s:%d", strings.ToLower(strings.TrimSpace(host)), port)
}

func splitGatewayKey(key string) (host string, port int) {
	host, portStr, ok := strings.Cut(key, ":")
	if !ok {
		return key, 443
	}
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		port = 443
	}
	return host, port
}

func statusForProfile(profile domain.VPNProfile, snap snapshot, peersOnGateway int) domain.VPNStatus {
	base := snap.gatewayStatus
	if base.State != domain.StateConnected {
		return base
	}

	target := resolveConnectionTarget(profile.VendorProfile, profile.Name, profile.Host, snap.saved)
	active := strings.TrimSpace(snap.activeConnection)

	if active != "" {
		if target != "" && strings.EqualFold(target, active) {
			return base
		}
		if target != "" {
			return domain.VPNStatus{
				State:  domain.StateDisconnected,
				Known:  true,
				Detail: fmt.Sprintf("Ivanti is connected with “%s”, not “%s”.", active, target),
			}
		}
		return domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: fmt.Sprintf("Ivanti is connected with “%s”. Pick the matching saved Ivanti connection on this card.", active),
		}
	}

	// Tunnel is up but the event log did not name the connection (rare).
	if peersOnGateway == 1 && target != "" {
		return base
	}
	if peersOnGateway == 1 {
		return base
	}
	return domain.VPNStatus{
		State:  domain.StateDisconnected,
		Known:  true,
		Detail: "Ivanti is up on this gateway; set the saved Ivanti connection on each card so Tobiro can tell them apart.",
	}
}

// resolveConnectionTarget maps a card to one Ivanti friendly-name.
func resolveConnectionTarget(vendorProfile, profileName, host string, saved []savedConnection) string {
	if v := strings.TrimSpace(vendorProfile); v != "" {
		return v
	}
	name := strings.TrimSpace(profileName)
	if name == "" {
		return ""
	}
	onHost := connectionsOnHost(saved, host)
	for _, c := range onHost {
		if strings.EqualFold(c.Name, name) {
			return c.Name
		}
	}
	var fuzzy []string
	for _, c := range onHost {
		if namesOverlap(name, c.Name) {
			fuzzy = append(fuzzy, c.Name)
		}
	}
	if len(fuzzy) == 1 {
		return fuzzy[0]
	}
	return ""
}

func connectionsOnHost(saved []savedConnection, host string) []savedConnection {
	host = strings.TrimSpace(host)
	if host == "" {
		return saved
	}
	var out []savedConnection
	for _, c := range saved {
		if strings.EqualFold(strings.TrimSpace(c.URI), host) {
			out = append(out, c)
		}
	}
	return out
}

func namesOverlap(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}
