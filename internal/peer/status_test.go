package peer

import (
	"testing"

	bctx "github.com/ncmink/biebie-protocol/context"

	"tobiro/internal/domain"
)

func plainly(s string) string { return s }

func sshProfile() domain.VPNProfile {
	return domain.VPNProfile{
		ID:       "p1",
		Kind:     domain.KindSSH,
		Provider: domain.ProviderSSH,
		Host:     "172.16.20.65",
		Port:     22,
		SSH:      domain.SSHSettings{User: "server1"},
	}
}

func liveTunnel() domain.VPNSession {
	return domain.VPNSession{
		ProfileID:  "p1",
		State:      domain.StateConnected,
		AssignedIP: domain.LoopbackAddress,
		Forwards: []domain.PortForward{
			{Name: "Kubernetes API", LocalPort: 16443, RemoteHost: "localhost", RemotePort: 6443},
			{LocalPort: 31538, RemoteHost: "localhost", RemotePort: 31539},
		},
	}
}

func TestAnUpTunnelPublishesItsForwards(t *testing.T) {
	status := statusFor("p1", sshProfile(), liveTunnel(), true, plainly)

	if !status.Connected {
		t.Fatalf("status = %+v, want connected", status)
	}
	if len(status.Forwards) != 2 {
		t.Fatalf("forwards = %+v, want both", status.Forwards)
	}

	// Without the gateway, a forward written against the far side's own
	// loopback names an address the reader already has, and matching it to a
	// cluster becomes guesswork.
	if status.Gateway != "172.16.20.65" {
		t.Errorf("gateway = %q, want the SSH server", status.Gateway)
	}

	forward, ok := status.LocalFor("172.16.20.65", 6443)
	if !ok {
		t.Fatal("a peer cannot find the API server in what was published")
	}
	if forward.Local() != "127.0.0.1:16443" {
		t.Errorf("local address = %q", forward.Local())
	}
}

// A peer that believed in a closed forward would send a cluster's traffic to a
// loopback port that either refuses it or now belongs to something else.
func TestForwardsAreWithheldWhileTheTunnelIsDown(t *testing.T) {
	down := []domain.VPNSession{
		{ProfileID: "p1", State: domain.StateDisconnected, Forwards: liveTunnel().Forwards},
		{ProfileID: "p1", State: domain.StateConnecting, Forwards: liveTunnel().Forwards},
		{ProfileID: "p1", State: domain.StateFailed, Forwards: liveTunnel().Forwards},
		{ProfileID: "p1", State: domain.StateDisconnecting, Forwards: liveTunnel().Forwards},
	}

	for _, session := range down {
		t.Run(string(session.State), func(t *testing.T) {
			status := statusFor("p1", sshProfile(), session, true, plainly)
			if len(status.Forwards) != 0 {
				t.Errorf("published %+v while %s", status.Forwards, session.State)
			}
			if status.Gateway != "" {
				t.Errorf("published gateway %q while %s", status.Gateway, session.State)
			}
			if _, ok := status.LocalFor("172.16.20.65", 6443); ok {
				t.Error("a peer would still route through a dead tunnel")
			}
		})
	}
}

// A VPN hands this machine a route, so there is nothing to substitute and the
// field stays off the wire entirely.
func TestAVPNPublishesNoForwards(t *testing.T) {
	profile := domain.VPNProfile{ID: "p2", Provider: domain.ProviderMock, Host: "vpn.example.test"}
	session := domain.VPNSession{ProfileID: "p2", State: domain.StateConnected, AssignedIP: "10.42.0.7"}

	status := statusFor("p2", profile, session, true, plainly)
	if status.Forwards != nil {
		t.Fatalf("forwards = %+v, want none", status.Forwards)
	}
	if status.AssignedIP != "10.42.0.7" {
		t.Errorf("assigned ip = %q", status.AssignedIP)
	}
}

func TestAProfileWithNoSessionIsReportedDown(t *testing.T) {
	status := statusFor("p1", sshProfile(), liveTunnel(), false, plainly)
	if status.State != bctx.AccessDisconnected || len(status.Forwards) != 0 {
		t.Fatalf("status = %+v, want a plain disconnected answer", status)
	}
}
