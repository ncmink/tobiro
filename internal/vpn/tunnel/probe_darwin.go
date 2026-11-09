package tunnel

import (
	"context"
	"net"
	"time"

	"tobiro/internal/sysexec"
)

const netstatPath = "/usr/sbin/netstat"

// Supported reports whether tunnel probing is implemented on this platform.
func Supported() bool { return true }

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
