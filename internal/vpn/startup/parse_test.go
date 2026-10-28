package startup

import (
	"reflect"
	"testing"
)

func TestParseDisabledMap(t *testing.T) {
	raw := `
	disabled services = {
		"io.tailscale.ipn.macsys.login-item-helper" => enabled
		"net.pulsesecure.pulseagent" => disabled
		"net.pulsesecure.CertService" => enabled
	}`
	got := parseDisabledMap(raw)
	want := map[string]bool{
		"io.tailscale.ipn.macsys.login-item-helper": false,
		"net.pulsesecure.pulseagent":                true,
		"net.pulsesecure.CertService":               false,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestMatchesName(t *testing.T) {
	names := []string{"Forcepoint NGFW VPN Client", "Stonesoft VPN Client"}
	if !matchesName(names, "forcepoint ngfw vpn client") {
		t.Fatal("expected a case-insensitive match")
	}
	if matchesName(names, "Tailscale") {
		t.Fatal("did not expect Tailscale to match")
	}
}
