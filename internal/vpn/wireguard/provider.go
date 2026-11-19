// Package wireguard adapts the official WireGuard app on macOS.
//
// Tunnels are registered with macOS as NetworkExtension VPN services. macOS
// publishes list, status, start and stop for those through scutil --nc, which
// is how Tobiro drives them without root and without ever reading .conf files
// or private keys.
//
// The wg and wg-quick tools from wireguard.com's quickstart need root and are
// not driven here. Linux and Windows report that WireGuard is not supported yet.
package wireguard

import (
	"tobiro/internal/domain"
)

const wireGuardAppPath = "/Applications/WireGuard.app"

// Provider is the WireGuard adapter.
type Provider struct{}

// New creates the adapter.
func New() *Provider { return &Provider{} }

// ID returns the provider key.
func (p *Provider) ID() domain.VPNProvider { return domain.ProviderWireGuard }
