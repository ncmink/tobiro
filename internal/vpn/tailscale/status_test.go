package tailscale

import (
	"testing"

	"tobiro/internal/domain"
)

func TestParseStatusRunning(t *testing.T) {
	raw := `{
		"BackendState": "Running",
		"TailscaleIPs": ["100.64.1.23", "fd7a:115c:a1e0::1"],
		"Self": {"TailscaleIPs": ["100.64.1.23"], "Online": true, "UserID": 42},
		"User": {"42": {"ID": 42, "LoginName": "ming@biebie.net", "DisplayName": "Ming"}}
	}`
	got := parseStatusJSON(raw).VPNStatus()
	if !got.Known || got.State != domain.StateConnected {
		t.Fatalf("state = %+v, want connected", got)
	}
	if got.AssignedIP != "100.64.1.23" {
		t.Fatalf("ip = %q, want 100.64.1.23", got.AssignedIP)
	}
	if got.Identity != "ming@biebie.net" {
		t.Fatalf("identity = %q, want ming@biebie.net", got.Identity)
	}
}

func TestParseStatusPrefersSelfIPv4(t *testing.T) {
	raw := `{
		"BackendState": "Running",
		"TailscaleIPs": ["fd7a:115c:a1e0::1"],
		"Self": {"TailscaleIPs": ["100.99.0.8", "fd7a:115c:a1e0::8"]}
	}`
	got := parseStatusJSON(raw)
	if got.AssignedIP != "100.99.0.8" {
		t.Fatalf("ip = %q, want 100.99.0.8", got.AssignedIP)
	}
}

func TestParseStatusStopped(t *testing.T) {
	got := parseStatusJSON(`{"BackendState":"Stopped"}`).VPNStatus()
	if !got.Known || got.State != domain.StateDisconnected {
		t.Fatalf("state = %+v, want disconnected", got)
	}
}

func TestParseStatusNeedsLogin(t *testing.T) {
	reading := parseStatusJSON(`{"BackendState":"NeedsLogin"}`)
	if !reading.NeedsLogin {
		t.Fatal("expected NeedsLogin")
	}
	got := reading.VPNStatus()
	if got.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected until signed in", got.State)
	}
}

func TestParseStatusSoleUserFallback(t *testing.T) {
	raw := `{
		"BackendState": "Running",
		"User": {"99": {"LoginName": "only@example.com"}}
	}`
	got := parseStatusJSON(raw)
	if got.Identity != "only@example.com" {
		t.Fatalf("identity = %q, want only@example.com", got.Identity)
	}
}

func TestParseStatusInvalidJSON(t *testing.T) {
	got := parseStatusJSON("not-json").VPNStatus()
	if got.Known {
		t.Fatal("unknown JSON must not clobber tracked state")
	}
}

func TestLooksLikeDaemonDown(t *testing.T) {
	if !looksLikeDaemonDown("Failed to connect to local Tailscale daemon") {
		t.Fatal("expected daemon-down detection")
	}
}
