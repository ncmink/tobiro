package doctor

import "testing"

func TestParseNetstatInet(t *testing.T) {
	raw := `Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
default            192.168.1.1        UGScg                 en0
0/1                10.168.6.160       UGSc                utun3
128.0/1            10.168.6.160       UGSc                utun3
127                127.0.0.1          UCS                   lo0
224.0.0/4          link#1             UmCS                  lo0

Internet6:
Destination                             Gateway                                 Flags               Netif Expire
default                                 fe80::%utun0                            UGc                 utun0
`
	routes := ParseNetstatInet(raw)
	if len(routes) < 4 {
		t.Fatalf("routes = %d, want at least 4: %+v", len(routes), routes)
	}
	if routes[0].Destination != "default" || routes[0].Netif != "en0" {
		t.Fatalf("default route = %+v", routes[0])
	}
	if routes[1].Destination != "0/1" || routes[1].Netif != "utun3" {
		t.Fatalf("low half = %+v", routes[1])
	}
}

func TestCapturesAllTrafficHalfDefault(t *testing.T) {
	routes := []InetRoute{
		{Destination: "default", Gateway: "192.168.1.1", Netif: "en0"},
		{Destination: "0/1", Gateway: "10.168.6.160", Netif: "utun3"},
		{Destination: "128.0/1", Gateway: "10.168.6.160", Netif: "utun3"},
	}
	if !CapturesAllTraffic(routes, "utun3", "10.168.6.160") {
		t.Fatal("0/1 + 128.0/1 via the tunnel must count as full tunnel")
	}
}

func TestCapturesAllTrafficDefaultViaTunnel(t *testing.T) {
	routes := []InetRoute{
		{Destination: "default", Gateway: "10.1.1.1", Netif: "utun4"},
	}
	if !CapturesAllTraffic(routes, "utun4", "10.1.1.1") {
		t.Fatal("default via the tunnel interface must count as full tunnel")
	}
}

func TestCapturesAllTrafficSplit(t *testing.T) {
	routes := []InetRoute{
		{Destination: "default", Gateway: "192.168.1.1", Netif: "en0"},
		{Destination: "172.31.171/24", Gateway: "10.168.6.160", Netif: "utun3"},
	}
	if CapturesAllTraffic(routes, "utun3", "10.168.6.160") {
		t.Fatal("a single RFC1918 route must not count as full tunnel")
	}
}
