package tailscale

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"tobiro/internal/domain"
)

// SessionReader supplies Biebie session state for picking one connected card.
type SessionReader interface {
	Get(profileID string) (domain.VPNSession, bool)
}

// ReconcileProfiles reads the daemon once and maps it to each Tailscale connection.
// Cards with a Tailscale account follow `tailscale switch` + tailnet; legacy cards
// without one share a single connected slot on the machine.
func ReconcileProfiles(
	ctx context.Context,
	provider *Provider,
	profiles []domain.VPNProfile,
	sessions SessionReader,
	apply func(domain.VPNProfile, domain.VPNStatus),
) {
	if len(profiles) == 0 || provider == nil || apply == nil {
		return
	}
	reading, sw, err := provider.ReadMachineStatus(ctx)
	if err != nil {
		return
	}
	if reading.BackendState == "" && reading.Detail != "" && !reading.NeedsLogin {
		return
	}

	allowEmpty := len(profiles) == 1
	type row struct {
		profile domain.VPNProfile
		status  domain.VPNStatus
	}
	rows := make([]row, 0, len(profiles))
	var legacyWouldConnect []domain.VPNProfile
	for _, p := range profiles {
		st := StatusFromReading(reading, p.VendorProfile, sw, allowEmpty)
		if st == nil || !st.Known {
			continue
		}
		vendorSet := strings.TrimSpace(p.VendorProfile) != ""
		if !vendorSet && !allowEmpty && reading.BackendState == "Running" {
			if s, ok := sessions.Get(p.ID); ok && s.State.IsActive() {
				st.State = domain.StateConnected
				if reading.AssignedIP != "" {
					st.AssignedIP = reading.AssignedIP
				}
				if reading.Identity != "" {
					st.Identity = reading.Identity
				}
				if st.Detail == "" {
					st.Detail = reading.Detail
				}
			}
		}
		rows = append(rows, row{p, *st})
		if !vendorSet && st.State == domain.StateConnected {
			legacyWouldConnect = append(legacyWouldConnect, p)
		}
	}
	winner := pickConnectedWinner(legacyWouldConnect, sessions)
	active := sw.ActiveLabel(reading.Identity)
	for _, r := range rows {
		st := r.status
		if strings.TrimSpace(r.profile.VendorProfile) != "" {
			apply(r.profile, st)
			continue
		}
		if st.State == domain.StateConnected && winner != "" && r.profile.ID != winner {
			st.State = domain.StateDisconnected
			if active != "" {
				st.Detail = fmt.Sprintf(
					"Tailscale is up on %s; set a Tailscale account on “%s” or connect it explicitly.",
					active, r.profile.Name,
				)
			} else {
				st.Detail = fmt.Sprintf(
					"Tailscale is up on this machine; set a Tailscale account on each card or connect “%s”.",
					winnerProfileName(legacyWouldConnect, winner),
				)
			}
		}
		apply(r.profile, st)
	}
}

func winnerProfileName(profiles []domain.VPNProfile, id string) string {
	for _, p := range profiles {
		if p.ID == id {
			return p.Name
		}
	}
	return "the active connection"
}

func pickConnectedWinner(candidates []domain.VPNProfile, sessions SessionReader) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0].ID
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	var bestID string
	var bestAt time.Time
	for _, p := range candidates {
		s, ok := sessions.Get(p.ID)
		if !ok || s.State != domain.StateConnected {
			continue
		}
		at := s.StartedAt
		if s.ConnectedAt != nil {
			at = *s.ConnectedAt
		}
		if bestID == "" || at.After(bestAt) {
			bestID = p.ID
			bestAt = at
		}
	}
	if bestID != "" {
		return bestID
	}
	for _, p := range candidates {
		if s, ok := sessions.Get(p.ID); ok && s.State.IsActive() {
			return p.ID
		}
	}
	return candidates[0].ID
}
