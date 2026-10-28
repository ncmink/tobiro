//go:build darwin

package macosvpn

import (
	"context"
	"os"
	"testing"
	"time"

	"tobiro/internal/domain"
	"tobiro/internal/vpn/nc"
)

// TestMacOSVPNAgainstLiveService checks the adapter against a real VPN service
// configured on this machine, without touching the connection:
//
//	BIEBIE_TEST_MACOS_VPN="VPN CAT" go test ./internal/vpn/macosvpn/ -v
func TestMacOSVPNAgainstLiveService(t *testing.T) {
	name := os.Getenv("BIEBIE_TEST_MACOS_VPN")
	if name == "" {
		t.Skip(`set BIEBIE_TEST_MACOS_VPN="<service name>" to check a real service`)
	}

	ctx := context.Background()
	provider := New()
	profile := domain.VPNProfile{Provider: domain.ProviderMacOSVPN, VendorProfile: name}

	info, err := provider.Detect(ctx)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	t.Logf("installed=%v version=%q", info.Installed, info.Version)
	t.Logf("detail=%q", info.Detail)
	if !info.Installed {
		t.Fatal("macOS always manages its own VPN services")
	}

	caps, err := provider.Capabilities(ctx)
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	if !caps.CanDisconnect || !caps.CanReadStatus {
		t.Fatalf("stopping and reading need no secret, so both should be reported: %+v", caps)
	}
	if caps.CanConnect {
		t.Fatal("connecting needs a shared secret this app does not hold")
	}
	if caps.SupportsPassword {
		t.Fatal("no credential is collected for this provider")
	}

	// The form offers these instead of asking for a name, so every one of them
	// has to be a name resolve() then accepts.
	options, err := provider.ListVendorProfiles(ctx)
	if err != nil {
		t.Fatalf("list vendor profiles: %v", err)
	}
	if len(options) == 0 {
		t.Fatal("expected at least the service under test to be offered")
	}
	offered := false
	for _, option := range options {
		t.Logf("offered %q — %s (%s)", option.Name, option.Detail, option.State)
		if option.Name == name {
			offered = true
		}
		if _, err := provider.resolve(ctx, domain.VPNProfile{
			Provider:      domain.ProviderMacOSVPN,
			VendorProfile: option.Name,
		}); err != nil {
			t.Errorf("offered %q but it does not resolve: %v", option.Name, err)
		}
	}
	if !offered {
		t.Errorf("%q is configured but was not offered", name)
	}

	service, err := provider.resolve(ctx, profile)
	if err != nil {
		t.Fatalf("resolve %q: %v", name, err)
	}
	t.Logf("resolved %q to %s (%s)", service.Name, service.ID, service.Kind)
	if !service.SystemManaged() {
		t.Fatalf("%q is owned by another app, not macOS", service.Name)
	}

	status, err := provider.Status(ctx, profile)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("state=%s ip=%q interface=%q known=%v", status.State, status.AssignedIP, status.Interface, status.Known)
	t.Logf("detail=%q", status.Detail)
	if !status.Known {
		t.Fatalf("macOS reports this service's state directly: %s", status.Detail)
	}

	// A connected L2TP or IKEv2 service publishes its address, unlike the
	// NetworkExtension providers that publish none.
	if status.State == domain.StateConnected && status.AssignedIP == "" {
		t.Error("expected macOS to publish an address for a connected service")
	}

	// An unknown name has to say what the machine does offer, since the user
	// cannot see the list from the Tobiro window.
	if _, err := provider.resolve(ctx, domain.VPNProfile{
		Provider:      domain.ProviderMacOSVPN,
		VendorProfile: "No Such Service",
	}); err == nil {
		t.Fatal("expected an unknown service name to be rejected")
	} else {
		t.Logf("unknown name says: %v", err)
	}
}

// TestMacOSVPNDisconnectAgainstLiveService checks the half of the control that
// needs no secret: stopping a tunnel someone else started.
//
// It needs a service that is already connected — connect it in System Settings
// first — and it leaves it disconnected, so it takes its own opt-in:
//
//	BIEBIE_TEST_MACOS_VPN="VPN CAT" BIEBIE_TEST_MACOS_VPN_DISCONNECT=1 \
//	  go test ./internal/vpn/macosvpn/ -run Disconnect -v -timeout 2m
func TestMacOSVPNDisconnectAgainstLiveService(t *testing.T) {
	name := os.Getenv("BIEBIE_TEST_MACOS_VPN")
	if name == "" || os.Getenv("BIEBIE_TEST_MACOS_VPN_DISCONNECT") == "" {
		t.Skip("set BIEBIE_TEST_MACOS_VPN_DISCONNECT=1 as well to disconnect a live tunnel")
	}

	ctx := context.Background()
	provider := New()
	profile := domain.VPNProfile{Provider: domain.ProviderMacOSVPN, VendorProfile: name}

	service, err := provider.resolve(ctx, profile)
	if err != nil {
		t.Fatalf("resolve %q: %v", name, err)
	}

	before, err := provider.Status(ctx, profile)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("state before: %s (%s)", before.State, before.Detail)
	if before.State != domain.StateConnected {
		t.Skipf("connect %q in System Settings first — this checks the disconnect path", name)
	}

	if err := provider.Disconnect(ctx, profile); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		reading, err := nc.Status(ctx, service.ID)
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		if !reading.Connected {
			t.Logf("disconnected, macOS reports %s", reading.State)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was still connected after 30s", service.Name)
		}
		time.Sleep(300 * time.Millisecond)
	}
}
