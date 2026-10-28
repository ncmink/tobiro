package macosvpn

import (
	"context"
	"errors"
	"strings"
	"testing"

	"tobiro/internal/domain"
	"tobiro/internal/vpn"
	"tobiro/internal/vpn/nc"
)

func TestProviderID(t *testing.T) {
	if got := New().ID(); got != domain.ProviderMacOSVPN {
		t.Fatalf("ID() = %s, want %s", got, domain.ProviderMacOSVPN)
	}
}

func TestConnectRefusesRatherThanFailingHalfway(t *testing.T) {
	if !nc.Supported() {
		t.Skip("macOS VPN services are only available on macOS")
	}

	// macOS answers "The IPSec Shared Secret is missing" for a service whose
	// secret it holds in the System keychain, so claiming Connect and then
	// failing there would be worse than declining with somewhere to go.
	caps, err := New().Capabilities(context.Background())
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !caps.Installed {
		t.Skip("no macOS VPN services on this machine")
	}
	if caps.CanConnect {
		t.Fatal("Connect needs a secret this app does not hold, so it must not be advertised")
	}
	if !caps.RequiresNativeUI || !caps.CanOpenNativeClient {
		t.Fatal("declining Connect is only acceptable with a way to reach System Settings")
	}
	// Stopping and reading need no secret, so they stay automatic.
	if !caps.CanDisconnect || !caps.CanReadStatus {
		t.Fatal("disconnect and status need no secret and should still be driven directly")
	}
	if caps.SupportsPassword {
		t.Fatal("no credential is collected for this provider")
	}
}

func TestConnectPointsAtSystemSettings(t *testing.T) {
	if !nc.Supported() {
		t.Skip("macOS VPN services are only available on macOS")
	}

	options, err := New().ListVendorProfiles(context.Background())
	if err != nil || len(options) == 0 {
		t.Skip("no macOS VPN services configured on this machine")
	}

	err = New().Connect(context.Background(), domain.VPNProfile{
		Provider:      domain.ProviderMacOSVPN,
		VendorProfile: options[0].Name,
	}, domain.Credential{})

	var unsupported *domain.UnsupportedOperationError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected a refusal the UI can turn into an action, got %v", err)
	}
	if !unsupported.NativeUIFallback {
		t.Fatal("the user can complete this in System Settings, so the fallback must be offered")
	}
	if !strings.Contains(unsupported.Reason, "System Settings") {
		t.Fatalf("the reason should say where to go: %s", unsupported.Reason)
	}
}

// listMixed carries services of both kinds, so the offer must exclude the ones
// belonging to other apps — macOS cannot start those, and they have their own
// adapters here.
const listMixed = `Available network connection services in the current set (*=enabled):
* (Connected)      36703C19-DC97-4505-9C2C-F0BFA609CFE7 VPN (io.tailscale.ipn.macsys) "Tailscale"                      [VPN:io.tailscale.ipn.macsys]
* (Connected)      21E566BB-76AE-4D64-AD57-B2B14E830217 PPP --> L2TP       "VPN CAT"                        [PPP:L2TP]
* (Disconnected)   8410E0F7-053D-4476-8F25-A53A74B48339 PPP --> L2TP       "Sweet Home"                     [PPP:L2TP]
* (Disconnected)   82D5DE1B-45F2-466A-8CF3-E8A7F3811925 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard"           [VPN:com.surfshark.vpnclient.macos.direct]
`

func TestOfferListsOnlyTheServicesMacOSCanStart(t *testing.T) {
	message := offer(nc.ParseList(listMixed))

	for _, name := range []string{"VPN CAT", "Sweet Home"} {
		if !strings.Contains(message, name) {
			t.Fatalf("expected %q to be offered: %s", name, message)
		}
	}
	for _, name := range []string{"Tailscale", "Surfshark"} {
		if strings.Contains(message, name) {
			t.Fatalf("%q is driven by its own app and must not be offered: %s", name, message)
		}
	}
}

func TestOfferSaysWhereToCreateOneWhenThereAreNone(t *testing.T) {
	message := offer(nc.ParseList("Available network connection services in the current set (*=enabled):\n"))
	if !strings.Contains(message, "System Settings") {
		t.Fatalf("a user with no services needs to be told where to add one: %s", message)
	}
}

func TestDescribeKindNamesTheProtocol(t *testing.T) {
	cases := map[string]string{
		"PPP:L2TP": "L2TP over IPsec",
		"PPP:PPTP": "PPTP",
		// macOS reports IKEv2 and Cisco IPsec identically, so both are named
		// rather than one being guessed at.
		"IPSec": "IKEv2 or Cisco IPsec",
		// An unrecognised type is passed through rather than hidden.
		"Something:New": "Something:New",
	}

	for kind, want := range cases {
		if got := describeKind(kind); got != want {
			t.Fatalf("describeKind(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestTheAdapterCanBeAskedWhatIsConfigured(t *testing.T) {
	// The form only offers a list for adapters implementing this, so losing the
	// method would silently fall back to asking the user to type a name.
	var _ vpn.VendorProfileLister = New()
}
