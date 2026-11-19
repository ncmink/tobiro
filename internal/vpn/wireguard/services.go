package wireguard

import (
	"strings"

	"tobiro/internal/vpn/nc"
)

// ownedByWireGuard keeps VPN services registered by the official WireGuard app.
//
// macOS attributes NetworkExtension tunnels by bundle id; both the App Store
// build (com.wireguard.macos) and other official builds include "wireguard".
func ownedByWireGuard(services []nc.Service) []nc.Service {
	var out []nc.Service
	for _, service := range services {
		if strings.Contains(strings.ToLower(service.ProviderID()), "wireguard") {
			out = append(out, service)
		}
	}
	return out
}

func tunnelNames(services []nc.Service) []string {
	var out []string
	for _, service := range services {
		out = append(out, service.Name)
	}
	return out
}

func offerTunnels(services []nc.Service) string {
	names := tunnelNames(services)
	if len(names) == 0 {
		return "Import a tunnel in the WireGuard app first so macOS registers it."
	}
	return "Registered tunnels: " + strings.Join(names, ", ") + "."
}
