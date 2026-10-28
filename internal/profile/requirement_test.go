package profile

import (
	"errors"
	"strings"
	"testing"

	"tobiro/internal/domain"
)

// tunnel creates a fully set up SSH profile, optionally behind the connection
// it needs first — a bastion inside the customer network, reachable only once
// their VPN is carrying the route.
func tunnel(t *testing.T, svc *Service, name, requires string) domain.VPNProfile {
	t.Helper()
	p, err := svc.Create(domain.ProfileInput{
		Name:     name,
		Kind:     domain.KindSSH,
		Host:     "172.16.20.65",
		Port:     22,
		Requires: requires,
		SSH: domain.SSHSettings{
			User:     "server1",
			Forwards: []domain.PortForward{{LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443}},
		},
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return p
}

func TestATunnelRemembersTheVPNItSitsBehind(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "SMOI")
	bastion := tunnel(t, svc, "SMOI bastion", vpn.ID)

	stored, err := svc.Get(bastion.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Requires != vpn.ID {
		t.Fatalf("expected the tunnel to need %s first, got %q", vpn.ID, stored.Requires)
	}
}

func TestAConnectionCannotNeedItself(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "")

	_, err := svc.SetRequirement(p.ID, p.ID)
	if err == nil {
		t.Fatal("expected a connection that needs itself to be refused")
	}
	if !strings.Contains(err.Error(), "itself") {
		t.Fatalf("expected the error to say so, got %v", err)
	}
}

func TestTwoConnectionsCannotNeedEachOther(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "")
	bastion := tunnel(t, svc, "SMOI bastion", vpn.ID)

	// Neither could ever be brought up, so this is refused at the point it is
	// described rather than at the point somebody clicks Connect.
	if _, err := svc.SetRequirement(vpn.ID, bastion.ID); err == nil {
		t.Fatal("expected a two-way requirement to be refused")
	}
}

func TestALongerLoopIsRefused(t *testing.T) {
	svc, _ := newService(t)
	first := create(t, svc, "Edge VPN", "")
	second := tunnel(t, svc, "Inner bastion", first.ID)
	third := tunnel(t, svc, "Deep bastion", second.ID)

	if _, err := svc.SetRequirement(first.ID, third.ID); err == nil {
		t.Fatal("expected a three-hop loop to be refused")
	}
}

func TestARequirementMustNameSomethingThatExists(t *testing.T) {
	svc, _ := newService(t)
	p := create(t, svc, "SMOI", "")

	_, err := svc.SetRequirement(p.ID, "1cbe0e0c-0000-0000-0000-000000000000")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected a missing prerequisite to be not-found, got %v", err)
	}
}

func TestDeletingAVPNReleasesTheTunnelsBehindIt(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "")
	bastion := tunnel(t, svc, "SMOI bastion", vpn.ID)

	if err := svc.Delete(vpn.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	stored, err := svc.Get(bastion.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Left pointing at a gap, the tunnel would refuse to start for a reason
	// nobody could see on screen.
	if stored.Requires != "" {
		t.Fatalf("expected the requirement to be cleared, got %q", stored.Requires)
	}
}

func TestDependentsFindsWhatWaitsOnAConnection(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "")
	first := tunnel(t, svc, "SMOI bastion", vpn.ID)
	second := tunnel(t, svc, "SMOI jump host", vpn.ID)
	tunnel(t, svc, "Somewhere else", "")

	dependents := svc.Dependents(vpn.ID)
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents, got %d: %+v", len(dependents), dependents)
	}
	found := map[string]bool{}
	for _, d := range dependents {
		found[d.ID] = true
	}
	if !found[first.ID] || !found[second.ID] {
		t.Fatalf("expected both tunnels, got %+v", dependents)
	}
}

func TestClearingARequirementLeavesTheConnectionStandingAlone(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "")
	bastion := tunnel(t, svc, "SMOI bastion", vpn.ID)

	if _, err := svc.SetRequirement(bastion.ID, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(svc.Dependents(vpn.ID)) != 0 {
		t.Fatalf("expected nothing left waiting on the VPN")
	}
}
