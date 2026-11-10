//go:build darwin || windows

package tunnel

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	probeTimeout = 5 * time.Second
	snapshotTTL  = time.Second
)

// snapshot is one capture of the machine's sockets and tunnel addresses.
//
// tcp and udp hold one connection per line in netstat-like form; each
// platform's collect fills them so the matching below stays shared.
type snapshot struct {
	tcp      string
	udp      string
	tunnels  []tunnelAddr
	captured time.Time
}

type tunnelAddr struct {
	ip, iface string
	// desc is the adapter description where the OS publishes one (Windows);
	// it names the vendor driver, e.g. "Juniper Networks Virtual Adapter".
	desc string
}

var (
	snapMu     sync.Mutex
	cachedSnap *snapshot
)

// ProbeWith reports whether a tunnel matching opts is currently up.
//
// Forcepoint (and similar SSL VPNs) mark the session Connected in their GUI as
// soon as the control TCP socket is established. The tunnel address is assigned
// afterwards. Requiring both at the same instant is what made the dashboard
// flicker or stay Disconnected. The rule is therefore:
//
//	control socket owned by the vendor client  →  connected
//	control socket + a tunnel address          →  connected (with IP)
//	vendor adapter up (AdapterHints)           →  connected (with IP)
//	a tunnel without a matching socket         →  not this profile
func ProbeWith(ctx context.Context, opts Options) Reading {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	snap := capture(ctx)
	adapter, hasAdapter := snap.vendorAdapter(opts.AdapterHints)
	if opts.RequireAdapter && !hasAdapter {
		return Reading{Detail: "The vendor virtual adapter is not up."}
	}

	gatewayIPs := resolveGateway(ctx, opts.Host)
	if len(gatewayIPs) == 0 {
		// An adapter alone cannot say whose tunnel it is, so a profile whose
		// gateway is unknown does not claim it.
		if hasAdapter && !opts.RequireAdapter {
			return adapterReading(adapter)
		}
		return Reading{Detail: "The gateway address could not be resolved."}
	}

	session, ownedByClient := snap.hasGatewaySession(gatewayIPs, opts.Port, opts.ProcessHints)
	ip, iface, hasTunnel := snap.assignedAddress()
	if hasAdapter {
		ip, iface, hasTunnel = adapter.ip, adapter.iface, true
	}

	if session && !ownedByClient && opts.Port != 80 && opts.Port != 443 {
		ownedByClient = true
	}

	if opts.RequireAdapter {
		if !session {
			return Reading{Detail: "The vendor adapter is up, but not for this gateway."}
		}
		return adapterReading(adapter)
	}

	switch {
	case session && ownedByClient:
		reading := Reading{Connected: true, AssignedIP: ip, Interface: iface}
		if iface != "" {
			reading.Detail = "Tunnel up on " + iface + " with address " + ip + "."
		} else {
			reading.Detail = "Vendor client is connected to the gateway."
		}
		return reading

	case session && hasTunnel:
		return Reading{
			Connected:  true,
			AssignedIP: ip,
			Interface:  iface,
			Detail:     "Tunnel up on " + iface + " with address " + ip + ".",
		}

	case hasAdapter:
		return adapterReading(adapter)

	case session:
		return Reading{Detail: "A connection to the gateway exists, but it is not the vendor VPN client."}

	default:
		return Reading{Detail: "No control connection to the gateway."}
	}
}

func adapterReading(a tunnelAddr) Reading {
	return Reading{
		Connected:  true,
		AssignedIP: a.ip,
		Interface:  a.iface,
		Detail:     "Tunnel up on " + a.iface + " with address " + a.ip + ".",
	}
}

func probeVendor(ctx context.Context, hints []string) Reading {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	snap := capture(ctx)
	if !snap.hasVendorSession(hints) {
		return Reading{Detail: "The vendor client has no live control connection."}
	}

	ip, iface, _ := snap.assignedAddress()
	reading := Reading{Connected: true, AssignedIP: ip, Interface: iface}
	if iface != "" && ip != "" {
		reading.Detail = "Tunnel up on " + iface + " with address " + ip + "."
		return reading
	}
	reading.Detail = "Vendor client is connected to its gateway."
	return reading
}

func (s *snapshot) hasVendorSession(hints []string) bool {
	if len(hints) == 0 {
		return false
	}
	for _, table := range []string{s.tcp, s.udp} {
		for _, line := range strings.Split(table, "\n") {
			if !lineOwnedBy(line, hints) {
				continue
			}
			if isTCPLine(line) && !strings.Contains(line, "ESTABLISHED") {
				continue
			}
			if isTCPLine(line) || strings.HasPrefix(strings.TrimSpace(line), "udp") || strings.Contains(line, "TCP ") {
				return true
			}
		}
	}
	return false
}

func invalidateSnapshot() {
	snapMu.Lock()
	cachedSnap = nil
	snapMu.Unlock()
}

func capture(ctx context.Context) *snapshot {
	snapMu.Lock()
	if cachedSnap != nil && time.Since(cachedSnap.captured) < snapshotTTL {
		snap := cachedSnap
		snapMu.Unlock()
		return snap
	}
	snapMu.Unlock()

	snap := collect(ctx)

	snapMu.Lock()
	cachedSnap = snap
	snapMu.Unlock()
	return snap
}

func resolveGateway(ctx context.Context, host string) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if net.ParseIP(host) != nil {
		return []string{host}
	}
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil
	}
	return addresses
}

func (s *snapshot) hasGatewaySession(gatewayIPs []string, port int, hints []string) (matched bool, ownedByClient bool) {
	portDot := "." + strconv.Itoa(port)
	portColon := ":" + strconv.Itoa(port)

	for _, ip := range gatewayIPs {
		for _, table := range []string{s.tcp, s.udp} {
			for _, line := range strings.Split(table, "\n") {
				if !strings.Contains(line, ip) {
					continue
				}
				if isTCPLine(line) && !strings.Contains(line, "ESTABLISHED") {
					continue
				}

				owned := lineOwnedBy(line, hints)
				exactPort := strings.Contains(line, ip+portDot) || strings.Contains(line, ip+portColon)

				if exactPort {
					matched = true
					if owned {
						return true, true
					}
					continue
				}

				// The vendor GUI is Connected as soon as it holds a socket to
				// this gateway, even if the tunnel uses a different port than
				// the one saved on the profile.
				if owned && (isTCPLine(line) || strings.HasPrefix(strings.TrimSpace(line), "udp") || strings.Contains(line, "TCP ")) {
					return true, true
				}
			}
		}
	}
	return matched, false
}

func isTCPLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "tcp") || strings.Contains(trimmed, " TCP ")
}

func lineOwnedBy(line string, hints []string) bool {
	if len(hints) == 0 {
		return false
	}
	lower := strings.ToLower(line)
	for _, hint := range hints {
		hint = strings.ToLower(strings.TrimSpace(hint))
		if hint == "" {
			continue
		}
		if strings.Contains(lower, hint) {
			return true
		}
		// lsof truncates COMMAND (Forcepoint → Forcepoin).
		for _, n := range []int{8, 9, 10} {
			if len(hint) >= n && strings.Contains(lower, hint[:n]) {
				return true
			}
		}
	}
	return false
}

func (s *snapshot) assignedAddress() (ip, iface string, ok bool) {
	if len(s.tunnels) == 1 {
		return s.tunnels[0].ip, s.tunnels[0].iface, true
	}
	if len(s.tunnels) > 1 {
		// Several tunnels are up; the address is left blank rather than guessed.
		return "", "", true
	}
	return "", "", false
}

// vendorAdapter returns the first tunnel address whose adapter description
// matches one of hints.
func (s *snapshot) vendorAdapter(hints []string) (tunnelAddr, bool) {
	for _, t := range s.tunnels {
		if t.desc == "" {
			continue
		}
		desc := strings.ToLower(t.desc)
		for _, hint := range hints {
			hint = strings.ToLower(strings.TrimSpace(hint))
			if hint != "" && strings.Contains(desc, hint) {
				return t, true
			}
		}
	}
	return tunnelAddr{}, false
}

// findAssignedAddress is kept for tests that inspect the local tunnel list.
func findAssignedAddress() (ip string, iface string, ok bool) {
	return (&snapshot{tunnels: listTunnelAddresses()}).assignedAddress()
}
