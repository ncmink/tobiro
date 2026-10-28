package ivanti

import (
	"net"
	"testing"
)

func TestParseRecoverySplit(t *testing.T) {
	raw := "2,0,172.31.171.0,24,10.168.6.160,1,1A,5,0\n2,0,1.1.254.50,32,192.168.1.1,0,E,3,0\n"
	routes := parseRecovery(raw)
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(routes))
	}
	if recoveryCapturesAll(routes) {
		t.Fatal("a /24 plus a gateway /32 must not count as full tunnel")
	}
	if hops := nextHopsOf(routes); len(hops) != 2 {
		t.Fatalf("next hops = %#v", hops)
	}
	gws := gatewaysOf(routes)
	if len(gws) != 1 || gws[0] != "1.1.254.50" {
		t.Fatalf("gateways = %v, want [1.1.254.50]", gws)
	}
}

func TestParseRecoveryFullDefault(t *testing.T) {
	raw := "2,0,0.0.0.0,0,10.168.6.160,1,1A,5,0\n2,0,1.1.254.50,32,192.168.1.1,0,E,3,0\n"
	if !recoveryCapturesAll(parseRecovery(raw)) {
		t.Fatal("0.0.0.0/0 must count as full tunnel")
	}
}

func TestParseRecoveryHalfDefaultPair(t *testing.T) {
	raw := "" +
		"2,0,0.0.0.0,1,10.168.6.160,1,1A,5,0\n" +
		"2,0,128.0.0.0,1,10.168.6.160,1,1A,5,0\n" +
		"2,0,1.1.254.50,32,192.168.1.1,0,E,3,0\n"
	if !recoveryCapturesAll(parseRecovery(raw)) {
		t.Fatal("0.0.0.0/1 + 128.0.0.0/1 must count as full tunnel")
	}
}

func TestParseRecoveryIgnoresJunk(t *testing.T) {
	if routes := parseRecovery("# comment\n\n2,0,not-an-ip,24,10.0.0.1\n"); len(routes) != 0 {
		t.Fatalf("got %+v", routes)
	}
	if net.ParseIP("10.0.0.1") == nil {
		t.Fatal("sanity")
	}
}
