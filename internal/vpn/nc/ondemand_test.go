package nc

import "testing"

const showWireGuard = `* (Connected)      1AA1CE37-1CA5-4B24-9C2A-85533AF3A9FB VPN (com.wireguard.macos) "Test"                           [VPN:com.wireguard.macos]
VPN <dictionary> {
  AuthenticationMethod : Password
  NEProviderBundleIdentifier : com.wireguard.macos.network-extension
  OnDemandEnabled : %s
  OnDemandRules : <array> {
    0 : <dictionary> {
      Action : Connect
    }
  }
  RemoteAddress : Unspecified
}
`

func TestParseOnDemandEnabled(t *testing.T) {
	cases := map[string]bool{"TRUE": true, "1": true, "FALSE": false, "0": false}
	for value, want := range cases {
		raw := replaceFirst(showWireGuard, "%s", value)
		if got := ParseOnDemandEnabled(raw); got != want {
			t.Fatalf("OnDemandEnabled : %s parsed as %v, want %v", value, got, want)
		}
	}
	if ParseOnDemandEnabled("VPN <dictionary> {\n}\n") {
		t.Fatal("a configuration without the key is not on demand")
	}
}

func replaceFirst(s, old, repl string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + repl + s[i+len(old):]
		}
	}
	return s
}
