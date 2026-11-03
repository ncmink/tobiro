package ivanti

import (
	"testing"

	"tobiro/internal/domain"
)

func TestResolveConnectionTarget(t *testing.T) {
	saved := []savedConnection{
		{Name: "GovBKK Mild", URI: "1.1.254.50"},
		{Name: "EGP", URI: "1.1.254.50"},
	}
	if got := resolveConnectionTarget("", "GovBKK", "1.1.254.50", saved); got != "GovBKK Mild" {
		t.Fatalf("GovBKK → %q, want GovBKK Mild", got)
	}
	if got := resolveConnectionTarget("", "EGP2", "1.1.254.50", saved); got != "EGP" {
		t.Fatalf("EGP2 → %q, want EGP", got)
	}
	if got := resolveConnectionTarget("EGP", "anything", "1.1.254.50", saved); got != "EGP" {
		t.Fatalf("vendor profile override = %q", got)
	}
}

func TestStatusForProfileSharedGateway(t *testing.T) {
	up := domain.VPNStatus{
		State:      domain.StateConnected,
		Known:      true,
		AssignedIP: "10.168.6.139",
	}
	snap := snapshot{
		gatewayStatus:    up,
		activeConnection: "EGP",
		saved: []savedConnection{
			{Name: "GovBKK Mild", URI: "1.1.254.50"},
			{Name: "EGP", URI: "1.1.254.50"},
		},
	}
	gov := domain.VPNProfile{Name: "GovBKK", Host: "1.1.254.50", Port: 443}
	egp := domain.VPNProfile{Name: "EGP2", Host: "1.1.254.50", Port: 443}

	if st := statusForProfile(egp, snap, 2); st.State != domain.StateConnected {
		t.Fatalf("EGP2 card should be connected, got %s", st.State)
	}
	if st := statusForProfile(gov, snap, 2); st.State != domain.StateDisconnected {
		t.Fatalf("GovBKK card should be disconnected, got %s (%s)", st.State, st.Detail)
	}
}
