package tailscale

import (
	"encoding/json"
	"fmt"
	"strings"

	"tobiro/internal/domain"
)

type userProfile struct {
	LoginName   string `json:"LoginName"`
	DisplayName string `json:"DisplayName"`
}

type statusPayload struct {
	BackendState string   `json:"BackendState"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Self         *struct {
		TailscaleIPs []string `json:"TailscaleIPs"`
		Online       bool     `json:"Online"`
		UserID       int64    `json:"UserID"`
	} `json:"Self"`
	User map[string]userProfile `json:"User"`
}

type statusReading struct {
	BackendState string
	AssignedIP   string
	Identity     string
	NeedsLogin   bool
	Detail       string
}

// MachineReading is a shared Tailscale daemon snapshot for several connections.
type MachineReading struct {
	BackendState string
	AssignedIP   string
	Identity     string
	NeedsLogin   bool
	Detail       string
}

func (r statusReading) machine() MachineReading {
	return MachineReading{
		BackendState: r.BackendState,
		AssignedIP:   r.AssignedIP,
		Identity:     r.Identity,
		NeedsLogin:   r.NeedsLogin,
		Detail:       r.Detail,
	}
}

func machineReading(m MachineReading) statusReading {
	return statusReading{
		BackendState: m.BackendState,
		AssignedIP:   m.AssignedIP,
		Identity:     m.Identity,
		NeedsLogin:   m.NeedsLogin,
		Detail:       m.Detail,
	}
}

func parseStatusJSON(raw string) statusReading {
	var payload statusPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return statusReading{Detail: "could not read Tailscale status"}
	}

	reading := statusReading{
		BackendState: payload.BackendState,
		AssignedIP:   firstIPv4(payload.TailscaleIPs),
		Identity:     loginName(payload),
	}
	if reading.AssignedIP == "" && payload.Self != nil {
		reading.AssignedIP = firstIPv4(payload.Self.TailscaleIPs)
	}

	switch payload.BackendState {
	case "NeedsLogin", "NeedsMachineAuth":
		reading.NeedsLogin = true
		reading.Detail = "Tailscale needs you to sign in."
	case "Stopped", "NoState":
		reading.Detail = "Tailscale is off."
	case "Starting":
		reading.Detail = "Tailscale is starting."
	case "Running":
		reading.Detail = "Tailscale is up."
	}
	return reading
}

func (r statusReading) VPNStatus() *domain.VPNStatus {
	return r.vpnStatusForProfile("", ActiveSwitch{})
}

// vpnStatusForProfile maps daemon state to a connection card. When vendorProfile
// is non-empty, Running counts as connected only if that resolved profile is selected.
func (r statusReading) vpnStatusForProfile(vendorProfile string, sw ActiveSwitch) *domain.VPNStatus {
	allowEmpty := len(sw.Accounts) <= 1
	return r.vpnStatusForProfileOptions(vendorProfile, sw, allowEmpty)
}

func (r statusReading) vpnStatusForProfileOptions(vendorProfile string, sw ActiveSwitch, allowEmptyVendorMatch bool) *domain.VPNStatus {
	if r.BackendState == "" && r.Detail != "" && !r.NeedsLogin {
		return &domain.VPNStatus{Known: false, Detail: r.Detail}
	}

	target := strings.TrimSpace(vendorProfile)
	state := domain.StateDisconnected
	detail := r.Detail
	var resolved resolvedProfile

	if r.BackendState == "Running" {
		switch {
		case target != "":
			resolved = resolveProfile(sw.Accounts, target)
			switch resolved.Result {
			case resolveFound:
				if resolved.Account.Selected {
					state = domain.StateConnected
				}
			case resolveAmbiguous:
				detail = fmt.Sprintf(
					"Several Tailscale profiles on this machine match %q (same login). Pick a nickname or tailnet on this connection.",
					target,
				)
			case resolveNotFound:
				detail = fmt.Sprintf("Tailscale profile %q is not on this machine.", target)
			}
		case allowEmptyVendorMatch:
			state = domain.StateConnected
		}
	}

	if r.BackendState == "Running" && target != "" && state == domain.StateDisconnected && resolved.Result == resolveFound {
		active := sw.ActiveLabel(r.Identity)
		card := optionNameForAccount(resolved.Account, sw.Accounts)
		if active != "" {
			detail = fmt.Sprintf("Tailscale is up on %s; this connection is %s.", active, card)
		} else {
			detail = fmt.Sprintf("Tailscale is up on another profile; this connection is %s.", card)
		}
	}

	return &domain.VPNStatus{
		Known:      true,
		State:      state,
		AssignedIP: r.AssignedIP,
		Identity:   r.Identity,
		Detail:     detail,
	}
}

func loginName(payload statusPayload) string {
	if payload.Self != nil && payload.User != nil {
		id := fmt.Sprint(payload.Self.UserID)
		if u, ok := payload.User[id]; ok {
			if name := strings.TrimSpace(u.LoginName); name != "" {
				return name
			}
			if name := strings.TrimSpace(u.DisplayName); name != "" {
				return name
			}
		}
	}
	if len(payload.User) == 1 {
		for _, u := range payload.User {
			if name := strings.TrimSpace(u.LoginName); name != "" {
				return name
			}
		}
	}
	return ""
}

func firstIPv4(ips []string) string {
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.Contains(ip, ":") {
			continue
		}
		return ip
	}
	return ""
}

func looksLikeDaemonDown(combined string) bool {
	msg := strings.ToLower(combined)
	return strings.Contains(msg, "failed to connect") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "tailscaled")
}
