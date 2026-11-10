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

type fakeSessions map[string]domain.VPNSession

func (f fakeSessions) Get(id string) (domain.VPNSession, bool) {
	s, ok := f[id]
	return s, ok
}

func TestSessionOwner(t *testing.T) {
	group := []domain.VPNProfile{{ID: "gov"}, {ID: "egp"}}

	if got := sessionOwner(group, fakeSessions{}); got != "" {
		t.Fatalf("no sessions → %q, want none", got)
	}
	one := fakeSessions{
		"egp": {State: domain.StateConnecting},
		"gov": {State: domain.StateDisconnected},
	}
	if got := sessionOwner(group, one); got != "egp" {
		t.Fatalf("one active session → %q, want egp", got)
	}
	two := fakeSessions{
		"egp": {State: domain.StateConnected},
		"gov": {State: domain.StateConnected},
	}
	if got := sessionOwner(group, two); got != "" {
		t.Fatalf("two active sessions → %q, want none", got)
	}

	rememberOpened(domain.VPNProfile{ID: "gov"})
	t.Cleanup(func() { rememberOpened(domain.VPNProfile{}) })
	if got := sessionOwner(group, fakeSessions{}); got != "gov" {
		t.Fatalf("last opened card → %q, want gov", got)
	}
}
