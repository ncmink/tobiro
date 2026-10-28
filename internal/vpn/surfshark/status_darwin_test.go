//go:build darwin

package surfshark

import (
	"context"
	"os"
	"strings"
	"testing"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nc"
)

// ncListL2TPConnected is `scutil --nc list` from a Mac where Surfshark is
// disconnected while an L2TP service and Tailscale are up. Reading the machine's
// interfaces instead of asking macOS reported this as Surfshark connected,
// because Surfshark's tunnel extension keeps running after a disconnect and
// something else owned the default route.
const ncListL2TPConnected = `Available network connection services in the current set (*=enabled):
* (Connected)      36703C19-DC97-4505-9C2C-F0BFA609CFE7 VPN (io.tailscale.ipn.macsys) "Tailscale"                      [VPN:io.tailscale.ipn.macsys]
* (Disconnected)   8042E2FA-D58E-4518-AB9F-C828C1FCF19E VPN (com.fortinet.forticlient.macos.vpn) "VPN 2"                          [VPN:com.fortinet.forticlient.macos.vpn]
* (Connected)      21E566BB-76AE-4D64-AD57-B2B14E830217 PPP --> L2TP       "VPN CAT"                        [PPP:L2TP]
* (Disconnected)   82D5DE1B-45F2-466A-8CF3-E8A7F3811925 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard"           [VPN:com.surfshark.vpnclient.macos.direct]
* (Disconnected)   CA647BD8-0E34-4BA9-BF63-8F7B4CDA8A89 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard®"          [VPN:com.surfshark.vpnclient.macos.direct]
`

// ncListSurfsharkConnected is the same machine with Surfshark up. The stale
// duplicate configuration an upgrade left behind stays disconnected.
const ncListSurfsharkConnected = `Available network connection services in the current set (*=enabled):
* (Connected)      21E566BB-76AE-4D64-AD57-B2B14E830217 PPP --> L2TP       "VPN CAT"                        [PPP:L2TP]
* (Disconnected)   82D5DE1B-45F2-466A-8CF3-E8A7F3811925 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard"           [VPN:com.surfshark.vpnclient.macos.direct]
* (Connected)      CA647BD8-0E34-4BA9-BF63-8F7B4CDA8A89 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard®"          [VPN:com.surfshark.vpnclient.macos.direct]
`

func TestAnotherVendorsTunnelIsNotSurfshark(t *testing.T) {
	services := ownedBySurfshark(nc.ParseList(ncListL2TPConnected))
	if len(services) != 2 {
		t.Fatalf("expected both Surfshark configurations to be found, got %d", len(services))
	}
	if _, ok := firstConnected(services); ok {
		t.Fatal("an L2TP and a Tailscale session must not read as a Surfshark connection")
	}
}

func TestSurfsharkConnectedIsAttributedToTheRightService(t *testing.T) {
	connected, ok := firstConnected(ownedBySurfshark(nc.ParseList(ncListSurfsharkConnected)))
	if !ok {
		t.Fatal("expected Surfshark to read as connected")
	}
	if connected.ID != "CA647BD8-0E34-4BA9-BF63-8F7B4CDA8A89" {
		t.Fatalf("picked the wrong service: %s", connected.ID)
	}
	if connected.Name != "Surfshark. WireGuard®" {
		t.Fatalf("expected the service name to be read, got %q", connected.Name)
	}
}

func TestOtherVendorsAreNotMistakenForSurfshark(t *testing.T) {
	for _, service := range ownedBySurfshark(nc.ParseList(ncListL2TPConnected)) {
		if !strings.HasPrefix(service.Name, "Surfshark") {
			t.Fatalf("%q is not a Surfshark service", service.Name)
		}
	}
	if len(ownedBySurfshark(nc.ParseList("Available network connection services:\n"))) != 0 {
		t.Fatal("a machine with no Surfshark configuration must yield no services")
	}
}

// sysextList is `systemextensionsctl list`, including the stale Surfshark entry
// left behind by an upgrade — it must not mask the approved one.
const sysextList = `--- com.apple.system_extension.network_extension
enabled	active	teamID	bundleID (version)	name	[state]
*	*	YHUG37CKN8	com.surfshark.vpnclient.macos.direct.PacketTunnel-WireGuard (4.28.1/4373)	Surfshark. WireGuard	[activated enabled]
*	*	W5364U7YZB	io.tailscale.ipn.macsys.network-extension (1.98.10/101.98.10)	Tailscale Network Extension	[activated enabled]
		YHUG37CKN8	com.surfshark.vpnclient.macos.direct.PacketTunnel-WireGuard (4.15.1/2271)	Surfshark. WireGuard	[terminated waiting to uninstall on reboot]
`

const sysextAwaitingApproval = `--- com.apple.system_extension.network_extension
enabled	active	teamID	bundleID (version)	name	[state]
	*	YHUG37CKN8	com.surfshark.vpnclient.macos.direct.PacketTunnel-WireGuard (4.28.1/4373)	Surfshark. WireGuard	[activated waiting for user]
`

func TestExtensionState(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want extensionApproval
	}{
		{"approved despite a stale entry", sysextList, extensionEnabled},
		{"waiting for the user", sysextAwaitingApproval, extensionNotApproved},
		{"surfshark absent", "--- com.apple.system_extension.network_extension\n", extensionUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extensionState(tc.raw); got != tc.want {
				t.Fatalf("extensionState = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSurfsharkAgainstLiveClient checks the adapter against the Surfshark
// install on this machine. Say which state to expect, then toggle Surfshark and
// run it again — the two runs together are what confirm the reading follows
// Surfshark's own tunnel and not some other VPN that happens to be up:
//
//	BIEBIE_TEST_SURFSHARK=connected    go test ./internal/vpn/surfshark/ -v
//	BIEBIE_TEST_SURFSHARK=disconnected go test ./internal/vpn/surfshark/ -v
func TestSurfsharkAgainstLiveClient(t *testing.T) {
	want := os.Getenv("BIEBIE_TEST_SURFSHARK")
	if want == "" {
		t.Skip("set BIEBIE_TEST_SURFSHARK=connected|disconnected to check the installed client")
	}

	ctx := context.Background()
	provider := New()

	info, err := provider.Detect(ctx)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	t.Logf("installed=%v version=%q path=%q", info.Installed, info.Version, info.Path)
	t.Logf("detail=%q", info.Detail)
	if !info.Installed {
		t.Skip("Surfshark is not installed on this machine")
	}

	services, err := nc.List(ctx)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	for _, service := range ownedBySurfshark(services) {
		t.Logf("service %s state=%s name=%q", service.ID, service.State, service.Name)
	}

	status, err := provider.Status(ctx, domain.VPNProfile{Provider: domain.ProviderSurfshark})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("state=%s ip=%q interface=%q known=%v", status.State, status.AssignedIP, status.Interface, status.Known)
	t.Logf("detail=%q", status.Detail)

	if !status.Known {
		t.Fatal("the tunnel state should be readable on macOS")
	}
	if got := string(status.State); got != want {
		t.Fatalf("expected Surfshark to read as %s, got %s: %s", want, got, status.Detail)
	}
}
