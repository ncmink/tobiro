package profile

import (
	"strings"
	"testing"

	"tobiro/internal/domain"
)

// A tunnel is named and filed in the dialog, then set up on its own page. The
// two halves are separate saves, and neither may disturb the other.
func TestATunnelIsCreatedEmptyAndSetUpAfterwards(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.Create(domain.ProfileInput{Name: "SMOI bastion", Kind: domain.KindSSH})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Port != domain.DefaultSSHPort {
		t.Fatalf("port = %d, want the SSH default", created.Port)
	}

	configured, err := svc.SetTunnel(created.ID, domain.TunnelInput{
		Host: "172.16.20.65",
		Port: 22,
		SSH: domain.SSHSettings{
			User:     "server1",
			Forwards: []domain.PortForward{{LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443}},
		},
	})
	if err != nil {
		t.Fatalf("set tunnel: %v", err)
	}
	if configured.Host != "172.16.20.65" || configured.SSH.User != "server1" {
		t.Fatalf("tunnel = %+v", configured)
	}
	if err := configured.ReadyToConnect(); err != nil {
		t.Fatalf("a configured tunnel is still not ready: %v", err)
	}
}

func TestEditingATunnelDoesNotDisturbItsServerOrPorts(t *testing.T) {
	svc, _ := newService(t)
	created := tunnel(t, svc, "SMOI bastion", "")

	// The dialog does not ask about the SSH half, so it sends none — and a
	// rename must not empty the connection.
	renamed, err := svc.Update(created.ID, domain.ProfileInput{
		Name: "SMOI bastion (Bangkok)",
		Kind: domain.KindSSH,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if renamed.Host != "172.16.20.65" || renamed.SSH.User != "server1" {
		t.Fatalf("the rename lost the tunnel: %+v", renamed)
	}
	if len(renamed.SSH.Forwards) != 1 {
		t.Fatalf("forwards = %+v", renamed.SSH.Forwards)
	}
}

func TestTurningAVPNIntoATunnelDoesNotInheritItsGateway(t *testing.T) {
	svc, _ := newService(t)
	created := create(t, svc, "SMOI", "")

	changed, err := svc.Update(created.ID, domain.ProfileInput{
		Name: "SMOI",
		Kind: domain.KindSSH,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	// vpn.example was a gateway. Carrying it over would leave a tunnel pointed
	// at somewhere it was never meant to dial.
	if changed.Host != "" {
		t.Fatalf("host = %q, want an empty one to fill in", changed.Host)
	}
	if changed.Port != domain.DefaultSSHPort {
		t.Fatalf("port = %d, want the SSH default", changed.Port)
	}
}

func TestOnlyATunnelHasATunnelToSet(t *testing.T) {
	svc, _ := newService(t)
	vpn := create(t, svc, "SMOI Production VPN", "")

	_, err := svc.SetTunnel(vpn.ID, domain.TunnelInput{Host: "172.16.20.65", Port: 22})
	if err == nil {
		t.Fatal("expected a VPN to refuse an SSH configuration")
	}
	if !strings.Contains(err.Error(), "not an SSH tunnel") {
		t.Fatalf("error = %v", err)
	}
}

func TestAHalfConfiguredTunnelSaves(t *testing.T) {
	svc, _ := newService(t)
	created := tunnel(t, svc, "SMOI bastion", "")

	// Server and login are enough to connect; forwards can be added later.
	saved, err := svc.SetTunnel(created.ID, domain.TunnelInput{
		Host: "172.16.20.65",
		Port: 22,
		SSH:  domain.SSHSettings{User: "server1"},
	})
	if err != nil {
		t.Fatalf("set tunnel: %v", err)
	}
	if err := saved.ReadyToConnect(); err != nil {
		t.Fatalf("expected a tunnel without forwards to be ready: %v", err)
	}
}
