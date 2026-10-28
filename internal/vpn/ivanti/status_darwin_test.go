package ivanti

import (
	"context"
	"os"
	"testing"

	"tobiro/internal/domain"
)

// TestReadStatusAgainstLiveClient exercises the parser against whatever the
// installed client has published on this machine. It is a no-op on machines
// without Ivanti, so it stays useful without becoming flaky.
func TestReadStatusAgainstLiveClient(t *testing.T) {
	if _, err := os.Stat(routeRecoveryPath); err != nil {
		t.Skipf("no Ivanti route-recovery file on this machine: %v", err)
	}

	reading, err := readTunnel()
	if err != nil {
		t.Fatalf("read tunnel: %v", err)
	}
	t.Logf("gateways=%v live=%v assignedIP=%s iface=%s",
		reading.gateways, reading.live, reading.assignedIP, reading.iface)

	if !reading.live {
		t.Log("no tunnel is currently up; connect in the Ivanti client to exercise the connected path")
		return
	}

	if reading.assignedIP == "" || reading.iface == "" {
		t.Fatal("a live tunnel must report both an assigned IP and an interface")
	}
	if len(reading.gateways) == 0 {
		t.Fatal("a live tunnel must expose at least one gateway host route")
	}

	// The profile host that matches the live gateway must be attributed, and an
	// unrelated host must not be.
	profile := domain.VPNProfile{Host: reading.gateways[0]}
	status := readStatus(context.Background(), profile)
	if !status.Known || status.State != domain.StateConnected {
		t.Fatalf("expected the matching profile to read as connected, got %+v", status)
	}
	if status.AssignedIP != reading.assignedIP {
		t.Fatalf("assigned IP = %q, want %q", status.AssignedIP, reading.assignedIP)
	}

	other := readStatus(context.Background(), domain.VPNProfile{Host: "203.0.113.99"})
	if other.Known {
		t.Fatalf("an unrelated gateway must not be attributed this tunnel, got %+v", other)
	}
}

func TestReadStatusIgnoresUnroutableNextHops(t *testing.T) {
	// The LAN gateway appears as a next hop but is never bound locally, so it
	// must not be mistaken for the tunnel address.
	candidates := map[string]struct{}{"192.0.2.254": {}}
	if _, _, ok := findLocalAddress(candidates); ok {
		t.Fatal("an address that is not on any interface must not be reported as live")
	}
}
