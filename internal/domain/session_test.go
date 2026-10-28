package domain

import "testing"

func TestCanTransition(t *testing.T) {
	cases := []struct {
		name string
		from ConnectionState
		to   ConnectionState
		want bool
	}{
		{"start dialling", StateDisconnected, StateConnecting, true},
		{"dial to auth", StateConnecting, StateAuthenticating, true},
		{"auth to otp", StateAuthenticating, StateWaitingForOTP, true},
		{"otp back to auth", StateWaitingForOTP, StateAuthenticating, true},
		{"otp to connected", StateWaitingForOTP, StateConnected, true},
		{"connected to disconnecting", StateConnected, StateDisconnecting, true},
		{"retry after failure", StateFailed, StateConnecting, true},
		{"same state is a no-op", StateConnected, StateConnected, true},
		{"observe an already-up tunnel", StateDisconnected, StateConnected, true},
		{"observe recovery after a failed probe", StateFailed, StateConnected, true},

		{"cannot ask for otp before dialling", StateDisconnected, StateWaitingForOTP, false},
		{"cannot reconnect while disconnecting", StateDisconnecting, StateConnected, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("CanTransition(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestCredentialStringMasksSecret(t *testing.T) {
	cred := Credential{ID: "cred-1", Label: "Ming", Username: "ming", Secret: "SuperSecret123"}

	rendered := cred.String()
	if contains(rendered, "SuperSecret123") {
		t.Fatalf("Credential.String() leaked the secret: %s", rendered)
	}
	if !contains(rendered, "secret=***") {
		t.Fatalf("Credential.String() should mark the secret as set: %s", rendered)
	}
}

func TestProvidersWithoutAGatewayDoNotRequireHost(t *testing.T) {
	for _, provider := range []VPNProvider{ProviderTailscale, ProviderSurfshark, ProviderMacOSVPN} {
		in := ProfileInput{Name: "No gateway", Provider: provider}
		if provider.VendorProfileField().Required {
			in.VendorProfile = "VPN CAT"
		}
		if err := in.Validate(); err != nil {
			t.Fatalf("%s profile without a gateway should be valid: %v", provider, err)
		}
		if in.Provider.EndpointLabel() == "" {
			t.Fatalf("%s has no gateway, so it needs a label to show in its place", provider)
		}
	}
}

func TestCatalogueMirrorsTheEndpointRule(t *testing.T) {
	for _, info := range SupportedProviders() {
		if info.RequiresEndpoint != info.ID.RequiresEndpoint() {
			t.Fatalf("%s: catalogue says requiresEndpoint=%v, rule says %v",
				info.ID, info.RequiresEndpoint, info.ID.RequiresEndpoint())
		}
		if !info.RequiresEndpoint && info.EndpointNote == "" {
			t.Fatalf("%s: the form needs a note explaining why no gateway is asked for", info.ID)
		}
		if info.VendorProfileField != info.ID.VendorProfileField() {
			t.Fatalf("%s: catalogue and rule disagree on the vendor profile field", info.ID)
		}
		if field := info.VendorProfileField; field.Required && field.Label == "" {
			t.Fatalf("%s: a required field needs a label to ask for it by", info.ID)
		}
	}
}

func TestAProviderThatConnectsByServiceNameRequiresIt(t *testing.T) {
	in := ProfileInput{Name: "Customer L2TP", Provider: ProviderMacOSVPN}

	err := in.Validate()
	if err == nil {
		t.Fatal("macOS VPN has no gateway to fall back on, so the service name cannot be optional")
	}
	if !contains(err.Error(), "macOS VPN service") {
		t.Fatalf("the error should name the field the user has to fill: %v", err)
	}

	in.VendorProfile = "VPN CAT"
	if err := in.Validate(); err != nil {
		t.Fatalf("naming the service should be enough: %v", err)
	}
}

func TestServiceNamesWithParenthesesAreAccepted(t *testing.T) {
	in := ProfileInput{Name: "Customer", Provider: ProviderMacOSVPN, VendorProfile: "Client (Prod)"}
	if err := in.Validate(); err != nil {
		t.Fatalf("macOS VPN services are routinely named this way: %v", err)
	}

	in.VendorProfile = "Client; rm -rf /"
	if err := in.Validate(); err == nil {
		t.Fatal("expected shell metacharacters to stay rejected")
	}
}

func TestProfileInputRejectsShellMetacharacters(t *testing.T) {
	in := ProfileInput{Name: "Evil", Provider: ProviderMock, Host: "vpn.example; rm -rf /", Port: 443}
	if err := in.Validate(); err == nil {
		t.Fatal("expected a host containing shell metacharacters to be rejected")
	}
}

func TestValidateExecutablePathRejectsRelativeAndUnsafe(t *testing.T) {
	for _, path := range []string{"vpnclient", "../vpnclient", "/usr/bin/vpn; rm -rf /", "/usr/bin/vpn`id`"} {
		if err := ValidateExecutablePath(path); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
	if err := ValidateExecutablePath("/Applications/FortiClient.app"); err != nil {
		t.Fatalf("expected an absolute path to be accepted: %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
