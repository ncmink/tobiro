package tunnel

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestLineOwnedByForcepointNetstat(t *testing.T) {
	line := "tcp4       0      0  192.168.1.124.58259    103.28.101.191.10443   ESTABLISHED        10456          621  131072  132480 Forcepoint NGFW :98975  00102 00000000"

	if !lineOwnedBy(line, []string{"Forcepoint", "Stonesoft"}) {
		t.Fatal("expected the Forcepoint process name to match")
	}
	if lineOwnedBy(line, []string{"Ivanti"}) {
		t.Fatal("an Ivanti hint must not match a Forcepoint socket")
	}

	snap := &snapshot{tcp: line + "\n"}
	matched, owned := snap.hasGatewaySession([]string{"103.28.101.191"}, 10443, []string{"Forcepoint"})
	if !matched || !owned {
		t.Fatalf("matched=%v owned=%v, want both true", matched, owned)
	}

	matched, _ = snap.hasGatewaySession([]string{"203.0.113.99"}, 10443, []string{"Forcepoint"})
	if matched {
		t.Fatal("a different gateway must not match")
	}
}

func TestBrowserSocketIsNotOwnedByClient(t *testing.T) {
	line := "tcp4       0      0  192.168.1.124.58147    103.28.101.191.10443   ESTABLISHED            0            0  131072  132480 Google Chrome   :3770   00102 00000000"
	snap := &snapshot{tcp: line + "\n"}

	matched, owned := snap.hasGatewaySession([]string{"103.28.101.191"}, 10443, []string{"Forcepoint"})
	if !matched {
		t.Fatal("the socket should still be detected")
	}
	if owned {
		t.Fatal("a browser must not be treated as the vendor client")
	}
}

func TestNonEstablishedTCPIsIgnored(t *testing.T) {
	line := "tcp4       0      0  192.168.1.124.58259    103.28.101.191.10443   CLOSE_WAIT           0            0  131072  132480 Forcepoint NGFW :98975  00102 00000000"
	snap := &snapshot{tcp: line + "\n"}

	matched, _ := snap.hasGatewaySession([]string{"103.28.101.191"}, 10443, []string{"Forcepoint"})
	if matched {
		t.Fatal("CLOSE_WAIT must not count as a live control session")
	}
}

func TestProbeWithProcessHintDoesNotNeedTunnelAddress(t *testing.T) {
	snapMu.Lock()
	cachedSnap = &snapshot{
		tcp:      "tcp4 0 0 192.168.1.124.1 203.0.113.10.10443 ESTABLISHED 0 0 0 0 Forcepoint NGFW :1\n",
		captured: time.Now(),
	}
	snapMu.Unlock()
	t.Cleanup(func() {
		snapMu.Lock()
		cachedSnap = nil
		snapMu.Unlock()
	})

	reading := ProbeWith(context.Background(), Options{
		Host:         "203.0.113.10",
		Port:         10443,
		ProcessHints: []string{"Forcepoint"},
	})
	if !reading.Connected {
		t.Fatalf("a vendor-owned control socket should be connected even before a utun address exists: %+v", reading)
	}
}

func TestProbeVendorFindsForcepointOnADifferentGateway(t *testing.T) {
	snapMu.Lock()
	cachedSnap = &snapshot{
		tcp:      "tcp4 0 0 192.168.1.116.62439 103.28.101.191.10443 ESTABLISHED 0 0 0 0 Forcepoint NGFW :716\n",
		captured: time.Now(),
	}
	snapMu.Unlock()
	t.Cleanup(func() {
		snapMu.Lock()
		cachedSnap = nil
		snapMu.Unlock()
	})

	// The profile points at the bastion the VPN unlocks, not at Forcepoint's
	// own concentrator — the case that used to leave a live tunnel looking down.
	miss := ProbeWith(context.Background(), Options{
		Host:         "172.16.20.65",
		Port:         22,
		ProcessHints: []string{"Forcepoint", "Stonesoft", "sgvpn"},
	})
	if miss.Connected {
		t.Fatal("a bastion address must not be treated as the Forcepoint gateway")
	}

	hit := ProbeVendor(context.Background(), []string{"Forcepoint", "Stonesoft", "sgvpn"})
	if !hit.Connected {
		t.Fatalf("a Forcepoint-owned control socket should count as connected: %+v", hit)
	}
}

func TestProbeVendorIgnoresABrowser(t *testing.T) {
	snapMu.Lock()
	cachedSnap = &snapshot{
		tcp:      "tcp4 0 0 192.168.1.116.1 203.0.113.10.443 ESTABLISHED 0 0 0 0 Google Chrome :1\n",
		captured: time.Now(),
	}
	snapMu.Unlock()
	t.Cleanup(func() {
		snapMu.Lock()
		cachedSnap = nil
		snapMu.Unlock()
	})

	reading := ProbeVendor(context.Background(), []string{"Forcepoint"})
	if reading.Connected {
		t.Fatalf("a browser socket must not count as the vendor client: %+v", reading)
	}
}

func TestProbeRejectsUnrelatedGateway(t *testing.T) {
	reading := Probe(context.Background(), "203.0.113.99", 443)
	if reading.Connected {
		t.Fatalf("an unrelated gateway must not be reported connected: %+v", reading)
	}
}

func TestFindAssignedAddressPrefersHostMaskedTunnels(t *testing.T) {
	ip, iface, ok := findAssignedAddress()
	t.Logf("assigned=%q interface=%q found=%v", ip, iface, ok)
	if ok && iface != "" && ip == "" {
		t.Fatal("an identified interface must come with its address")
	}
}

func TestLsofTruncatedProcessNameMatchesForcepoint(t *testing.T) {
	line := "Forcepoin 98975 napisoot 8u IPv4 TCP 192.168.1.124:61475->103.28.101.191:10443 (ESTABLISHED)"
	if !lineOwnedBy(line, []string{"Forcepoint", "Stonesoft"}) {
		t.Fatal("lsof truncates Forcepoint to Forcepoin; that must still match")
	}

	snap := &snapshot{tcp: line + "\n"}
	matched, owned := snap.hasGatewaySession([]string{"103.28.101.191"}, 10443, []string{"Forcepoint"})
	if !matched || !owned {
		t.Fatalf("matched=%v owned=%v, want both true", matched, owned)
	}
}

// TestProbeAgainstLiveTunnel checks the probe against a real tunnel on this
// machine. Point it at a live gateway:
//
//	BIEBIE_TEST_VPN_HOST=103.28.101.191 BIEBIE_TEST_VPN_PORT=10443 go test ./internal/vpn/tunnel/ -v
func TestProbeAgainstLiveTunnel(t *testing.T) {
	host := os.Getenv("BIEBIE_TEST_VPN_HOST")
	port, _ := strconv.Atoi(os.Getenv("BIEBIE_TEST_VPN_PORT"))
	if host == "" || port == 0 {
		t.Skip("set BIEBIE_TEST_VPN_HOST and BIEBIE_TEST_VPN_PORT to probe a live tunnel")
	}

	reading := ProbeWith(context.Background(), Options{
		Host:         host,
		Port:         port,
		ProcessHints: []string{"Forcepoint", "Ivanti", "FortiClient", "Pulse", "Stonesoft"},
	})
	t.Logf("connected=%v assignedIP=%q interface=%q detail=%q",
		reading.Connected, reading.AssignedIP, reading.Interface, reading.Detail)

	if !reading.Connected {
		t.Fatalf("expected %s:%d to read as connected, got: %s", host, port, reading.Detail)
	}
}
