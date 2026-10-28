package tunnel

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"tobiro/internal/sysexec"
)

const (
	netstatPath  = "/usr/sbin/netstat"
	probeTimeout = 5 * time.Second
	snapshotTTL  = time.Second
)

// Supported reports whether tunnel probing is implemented on this platform.
func Supported() bool { return true }

type snapshot struct {
	tcp      string
	udp      string
	tunnels  []tunnelAddr
	captured time.Time
}

type tunnelAddr struct {
	ip, iface string
}

var (
	snapMu     sync.Mutex
	cachedSnap *snapshot
)

// ProbeWith reports whether a tunnel matching opts is currently up.
//
// Forcepoint (and similar SSL VPNs) mark the session Connected in their GUI as
// soon as the control TCP socket is established. The utun address is assigned
// afterwards. Requiring both at the same instant is what made the dashboard
// flicker or stay Disconnected. The rule is therefore:
//
//	control socket owned by the vendor client  →  connected
//	control socket + a /32 tunnel address      →  connected (with IP)
//	a /32 tunnel without a matching socket     →  not this profile
func ProbeWith(ctx context.Context, opts Options) Reading {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	gatewayIPs := resolveGateway(ctx, opts.Host)
	if len(gatewayIPs) == 0 {
		return Reading{Detail: "The gateway address could not be resolved."}
	}

	snap := capture(ctx)
	session, ownedByClient := snap.hasGatewaySession(gatewayIPs, opts.Port, opts.ProcessHints)
	ip, iface, hasTunnel := snap.assignedAddress()

	if session && !ownedByClient && opts.Port != 80 && opts.Port != 443 {
		ownedByClient = true
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

	case session:
		return Reading{Detail: "A connection to the gateway exists, but it is not the vendor VPN client."}

	default:
		return Reading{Detail: "No control connection to the gateway."}
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

func collect(ctx context.Context) *snapshot {
	snap := &snapshot{captured: time.Now(), tunnels: listTunnelAddresses()}
	if res, err := sysexec.Run(ctx, netstatPath, "-anv", "-p", "tcp"); err == nil || res.Stdout != "" {
		snap.tcp = res.Stdout
	} else if res, err := sysexec.Run(ctx, netstatPath, "-an", "-p", "tcp"); err == nil || res.Stdout != "" {
		snap.tcp = res.Stdout
	}
	if res, err := sysexec.Run(ctx, netstatPath, "-anv", "-p", "udp"); err == nil || res.Stdout != "" {
		snap.udp = res.Stdout
	}
	if res, err := sysexec.Run(ctx, "/usr/sbin/lsof", "-nP", "-iTCP", "-sTCP:ESTABLISHED"); err == nil || res.Stdout != "" {
		snap.tcp += "\n" + res.Stdout
	}
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

func listTunnelAddresses() []tunnelAddr {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var out []tunnelAddr
	for _, candidate := range interfaces {
		const wanted = net.FlagUp | net.FlagPointToPoint
		if candidate.Flags&wanted != wanted || candidate.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := candidate.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			netIP, valid := addr.(*net.IPNet)
			if !valid || netIP.IP.To4() == nil {
				continue
			}
			if netIP.IP.IsLinkLocalUnicast() {
				continue
			}
			ones, bits := netIP.Mask.Size()
			if ones != bits {
				continue
			}
			out = append(out, tunnelAddr{ip: netIP.IP.String(), iface: candidate.Name})
		}
	}
	return out
}

// findAssignedAddress is kept for tests that inspect the local tunnel list.
func findAssignedAddress() (ip string, iface string, ok bool) {
	return (&snapshot{tunnels: listTunnelAddresses()}).assignedAddress()
}
