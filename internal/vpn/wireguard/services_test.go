package wireguard

import (
	"strings"
	"testing"

	"tobiro/internal/vpn/nc"
)

// listWithWireGuard mixes WireGuard, Tailscale, Surfshark and macOS L2TP entries.
const listWithWireGuard = `Available network connection services in the current set (*=enabled):
* (Connected)      36703C19-DC97-4505-9C2C-F0BFA609CFE7 VPN (io.tailscale.ipn.macsys) "Tailscale"                      [VPN:io.tailscale.ipn.macsys]
* (Disconnected)   A1B2C3D4-E5F6-7890-ABCD-EF1234567890 VPN (com.wireguard.macos) "Home WG"                           [VPN:com.wireguard.macos]
* (Connected)      B2C3D4E5-F6A7-8901-BCDE-F12345678901 VPN (com.wireguard.macos) "Office VPN"                       [VPN:com.wireguard.macos]
* (Disconnected)   82D5DE1B-45F2-466A-8CF3-E8A7F3811925 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard" [VPN:com.surfshark.vpnclient.macos.direct]
* (Disconnected)   21E566BB-76AE-4D64-AD57-B2B14E830217 PPP --> L2TP       "VPN CAT"                        [PPP:L2TP]
`

func TestOwnedByWireGuardIgnoresOtherVendors(t *testing.T) {
	services := nc.ParseList(listWithWireGuard)
	tunnels := ownedByWireGuard(services)
	if len(tunnels) != 2 {
		t.Fatalf("expected 2 WireGuard tunnels, got %d", len(tunnels))
	}
	for _, tunnel := range tunnels {
		if !strings.Contains(strings.ToLower(tunnel.ProviderID()), "wireguard") {
			t.Fatalf("unexpected owner %q", tunnel.ProviderID())
		}
		if strings.Contains(strings.ToLower(tunnel.ProviderID()), "surfshark") {
			t.Fatalf("surfshark tunnel must not be attributed to WireGuard")
		}
	}
}

func TestOfferTunnelsListsNames(t *testing.T) {
	services := nc.ParseList(listWithWireGuard)
	msg := offerTunnels(ownedByWireGuard(services))
	if !strings.Contains(msg, "Home WG") || !strings.Contains(msg, "Office VPN") {
		t.Fatalf("offer should list tunnel names: %s", msg)
	}
}

func TestFindByNameAmongWireGuardTunnelsOnly(t *testing.T) {
	services := nc.ParseList(listWithWireGuard)
	tunnels := ownedByWireGuard(services)

	if _, ok := nc.FindByName(tunnels, "Tailscale"); ok {
		t.Fatal("Tailscale must not resolve as a WireGuard tunnel")
	}
	svc, ok := nc.FindByName(tunnels, "home wg")
	if !ok || svc.Name != "Home WG" {
		t.Fatalf("expected case-insensitive match for Home WG, got %+v ok=%v", svc, ok)
	}
}
