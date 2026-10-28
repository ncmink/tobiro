package tunnel

import (
	"context"

	"tobiro/internal/domain"
)

// Reading is what could be established about a profile's tunnel.
type Reading struct {
	Connected  bool
	AssignedIP string
	Interface  string
	Detail     string
}

// Options tunes how a profile is attributed to live network state.
type Options struct {
	Host string
	Port int

	// ProcessHints are case-insensitive substrings of the vendor client
	// process name, as published by netstat. A control connection owned by
	// one of these processes is enough to call the tunnel up — the assigned
	// address often appears a few seconds later.
	ProcessHints []string
}

// Probe reports whether a tunnel to host:port is currently up.
func Probe(ctx context.Context, host string, port int) Reading {
	return ProbeWith(ctx, Options{Host: host, Port: port})
}

// ProbeVendor reports whether the named vendor client holds any live control
// socket, without asking which gateway it is talking to.
//
// The host saved on a Forcepoint profile is often the machine behind the VPN
// (a bastion, an API server) rather than the concentrator the client actually
// dials. Matching only that host then leaves a perfectly live tunnel looking
// disconnected. A socket owned by the vendor process is the client's own word
// that it is up.
func ProbeVendor(ctx context.Context, hints []string) Reading {
	return probeVendor(ctx, hints)
}

// Invalidate drops any cached network snapshot so the next probe reads live
// sockets and interfaces. Call this when the user has just opened a vendor
// client — the 1s cache would otherwise hide the new control connection.
func Invalidate() {
	invalidateSnapshot()
}

// Status converts a probe reading into the adapter status shape.
func Status(reading Reading) *domain.VPNStatus {
	if !reading.Connected {
		return &domain.VPNStatus{
			State:  domain.StateDisconnected,
			Known:  true,
			Detail: reading.Detail,
		}
	}
	return &domain.VPNStatus{
		State:      domain.StateConnected,
		AssignedIP: reading.AssignedIP,
		Interface:  reading.Interface,
		Known:      true,
		Detail:     reading.Detail,
	}
}
