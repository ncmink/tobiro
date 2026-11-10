//go:build darwin || windows

package tunnel

import (
	"context"
	"testing"
	"time"
)

func withSnapshot(t *testing.T, snap *snapshot) {
	t.Helper()
	snap.captured = time.Now().Add(time.Hour)
	snapMu.Lock()
	cachedSnap = snap
	snapMu.Unlock()
	t.Cleanup(func() {
		snapMu.Lock()
		cachedSnap = nil
		snapMu.Unlock()
	})
}

var ivantiWindows = Options{
	Host:           "1.1.254.50",
	Port:           443,
	ProcessHints:   []string{"dsAccessService", "dsNcService", "Pulse", "Ivanti"},
	AdapterHints:   []string{"Juniper Networks Virtual Adapter", "Pulse Secure", "Ivanti"},
	RequireAdapter: true,
}

const idlePulseSocket = "tcp4 192.168.1.20:50123 1.1.254.50:443 ESTABLISHED 4120 PulseSecureService.exe\n"

func TestRequireAdapterIgnoresIdleVendorSocket(t *testing.T) {
	withSnapshot(t, &snapshot{tcp: idlePulseSocket})

	if reading := ProbeWith(context.Background(), ivantiWindows); reading.Connected {
		t.Fatalf("an idle Pulse service socket without the adapter must not be connected: %+v", reading)
	}
}

func TestRequireAdapterNeedsThisGateway(t *testing.T) {
	withSnapshot(t, &snapshot{
		tcp:     "tcp4 192.168.1.20:50123 203.0.113.7:443 ESTABLISHED 4120 dsAccessService.exe\n",
		tunnels: []tunnelAddr{{ip: "10.8.0.4", iface: "Ethernet 3", desc: "Juniper Networks Virtual Adapter"}},
	})

	if reading := ProbeWith(context.Background(), ivantiWindows); reading.Connected {
		t.Fatalf("a tunnel to another gateway must not light this card: %+v", reading)
	}
}

func TestRequireAdapterUnknownGatewayStaysDisconnected(t *testing.T) {
	withSnapshot(t, &snapshot{
		tcp:     idlePulseSocket,
		tunnels: []tunnelAddr{{ip: "10.8.0.4", iface: "Ethernet 3", desc: "Juniper Networks Virtual Adapter"}},
	})

	opts := ivantiWindows
	opts.Host = ""
	if reading := ProbeWith(context.Background(), opts); reading.Connected {
		t.Fatalf("a card with no gateway must not claim someone else's tunnel: %+v", reading)
	}
}

// Without RequireAdapter (Forcepoint, FortiClient) the vendor control socket is
// still enough, so those providers keep behaving as before.
func TestWithoutRequireAdapterControlSocketStillCounts(t *testing.T) {
	withSnapshot(t, &snapshot{tcp: idlePulseSocket})

	opts := ivantiWindows
	opts.RequireAdapter = false
	if reading := ProbeWith(context.Background(), opts); !reading.Connected {
		t.Fatalf("a vendor-owned control socket should still count for other providers: %+v", reading)
	}
}

func TestRequireAdapterConnectedWithAdapterAndSocket(t *testing.T) {
	withSnapshot(t, &snapshot{
		tcp:     idlePulseSocket,
		tunnels: []tunnelAddr{{ip: "10.8.0.4", iface: "Ethernet 3", desc: "Juniper Networks Virtual Adapter"}},
	})

	reading := ProbeWith(context.Background(), ivantiWindows)
	if !reading.Connected || reading.AssignedIP != "10.8.0.4" {
		t.Fatalf("adapter up plus a gateway socket should be connected: %+v", reading)
	}
}
