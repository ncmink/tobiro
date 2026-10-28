package ivanti

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/tunnel"
)

// statusSupported reports that macOS status reading is implemented, so
// Capabilities can advertise it.
func statusSupported() bool { return true }

// readStatus reports the live tunnel state for a profile.
//
// This reads the client's own published state — it does not drive any
// undocumented command. When the state cannot be established the status is
// returned with Known=false so the session manager keeps whatever it already
// knows rather than acting on a guess.
func readStatus(ctx context.Context, profile domain.VPNProfile) *domain.VPNStatus {
	reading, err := readTunnel()
	if err != nil {
		// The route-recovery file is the precise source, but it only exists
		// once the client has brought a tunnel up at least once. Fall back to
		// the shared probe so status still works on a fresh install.
		return genericStatus(ctx, profile)
	}

	if !reading.live {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: reading.describe(),
		}
	}

	if !reading.belongsTo(ctx, profile.Host) {
		// A tunnel is up, but the recovery file attributes it to a different
		// gateway. Confirm against the live control connection before
		// concluding this profile is down.
		return genericStatus(ctx, profile)
	}

	return &domain.VPNStatus{
		State:      domain.StateConnected,
		AssignedIP: reading.assignedIP,
		Interface:  reading.iface,
		Known:      true,
		Detail:     reading.describe(),
	}
}

// genericStatus falls back to the shared, vendor-neutral tunnel probe.
func genericStatus(ctx context.Context, profile domain.VPNProfile) *domain.VPNStatus {
	return tunnel.Status(tunnel.ProbeWith(ctx, tunnel.Options{
		Host:         profile.Host,
		Port:         profile.Port,
		ProcessHints: []string{"Ivanti", "Pulse", "pulsesvc", "Jam"},
	}))
}

// routeRecoveryPath is the file the macOS client maintains while a tunnel is
// up, so it can restore the routing table afterwards. It is world-readable and
// plain text, and it is the only place the client publishes the live tunnel to
// other processes.
//
// Each line is a route: 2,0,<destination>,<prefix>,<next hop>,...
//
//	2,0,172.31.171.0,24,10.168.6.160,1,1A,5,0   split route via the assigned IP
//	2,0,1.1.254.50,32,192.168.1.1,0,E,3,0       host route to the VPN gateway
const routeRecoveryPath = "/Library/Application Support/Pulse Secure/Pulse/RMrecovery-VPN.dat"

// tunnelReading is what could be established about the live tunnel.
type tunnelReading struct {
	// gateways are the /32 destinations in the recovery file, i.e. the VPN
	// gateways currently routed around the tunnel. Used to decide which
	// profile a tunnel belongs to.
	gateways []string

	assignedIP string
	iface      string

	// live is true only when an address from the recovery file was also found
	// on a local interface, which is what proves the tunnel is actually up
	// rather than the file being left over from a previous session.
	live bool
}

// readTunnel parses the client's route-recovery file and cross-checks it
// against the machine's real interfaces.
//
// The file alone is not trusted: it can survive an unclean disconnect. The
// authoritative signal is whether one of its next-hop addresses is currently
// bound to a local interface.
func readTunnel() (tunnelReading, error) {
	raw, err := os.ReadFile(routeRecoveryPath)
	if err != nil {
		return tunnelReading{}, err
	}

	var reading tunnelReading
	routes := parseRecovery(string(raw))
	reading.gateways = gatewaysOf(routes)
	nextHops := nextHopsOf(routes)
	if len(nextHops) == 0 {
		return reading, nil
	}

	if ip, iface, ok := findLocalAddress(nextHops); ok {
		reading.assignedIP = ip
		reading.iface = iface
		reading.live = true
	}
	return reading, nil
}

// findLocalAddress reports which of the candidate addresses is bound to a local
// interface. The tunnel's assigned IP is a next hop in the recovery file and is
// also configured on the tunnel interface; the LAN gateway is a next hop too
// but is never local, so this cleanly separates the two.
func findLocalAddress(candidates map[string]struct{}) (ip string, iface string, ok bool) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", false
	}

	for _, candidate := range interfaces {
		if candidate.Flags&net.FlagUp == 0 || candidate.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := candidate.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			netIP, valid := addr.(*net.IPNet)
			if !valid {
				continue
			}
			if _, match := candidates[netIP.IP.String()]; match {
				return netIP.IP.String(), candidate.Name, true
			}
		}
	}
	return "", "", false
}

// belongsTo reports whether the live tunnel terminates at the profile's host.
//
// Profiles are usually configured with the gateway IP, which matches directly.
// A hostname is resolved once, with a short timeout, so a slow DNS server
// cannot stall the status poll.
func (r tunnelReading) belongsTo(ctx context.Context, host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || len(r.gateways) == 0 {
		return false
	}
	for _, gateway := range r.gateways {
		if strings.EqualFold(gateway, host) {
			return true
		}
	}
	if net.ParseIP(host) != nil {
		return false
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	resolved, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	if err != nil {
		return false
	}
	for _, address := range resolved {
		for _, gateway := range r.gateways {
			if address == gateway {
				return true
			}
		}
	}
	return false
}

func (r tunnelReading) describe() string {
	if !r.live {
		return "No Ivanti tunnel is currently up."
	}
	return fmt.Sprintf("Tunnel up on %s with address %s.", r.iface, r.assignedIP)
}
