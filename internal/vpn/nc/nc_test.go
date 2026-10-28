package nc

import "testing"

// listMixed is a real `scutil --nc list` capture from a Mac carrying both kinds
// of service: four configured in System Settings and three installed by apps.
const listMixed = `Available network connection services in the current set (*=enabled):
* (Connected)      36703C19-DC97-4505-9C2C-F0BFA609CFE7 VPN (io.tailscale.ipn.macsys) "Tailscale"                      [VPN:io.tailscale.ipn.macsys]
* (Disconnected)   8042E2FA-D58E-4518-AB9F-C828C1FCF19E VPN (com.fortinet.forticlient.macos.vpn) "VPN 2"                          [VPN:com.fortinet.forticlient.macos.vpn]
* (Connected)      21E566BB-76AE-4D64-AD57-B2B14E830217 PPP --> L2TP       "VPN CAT"                        [PPP:L2TP]
* (Disconnected)   8410E0F7-053D-4476-8F25-A53A74B48339 PPP --> L2TP       "Sweet Home"                     [PPP:L2TP]
* (Disconnected)   82D5DE1B-45F2-466A-8CF3-E8A7F3811925 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard"           [VPN:com.surfshark.vpnclient.macos.direct]
* (Disconnected)   8BF83E0F-B7E2-4E44-9E89-772910B670D0 PPP --> L2TP       "VPN"                            [PPP:L2TP]
* (Disconnected)   CA647BD8-0E34-4BA9-BF63-8F7B4CDA8A89 VPN (com.surfshark.vpnclient.macos.direct) "Surfshark. WireGuard®"          [VPN:com.surfshark.vpnclient.macos.direct]
`

func TestParseListSkipsTheHeaderAndReadsEveryColumn(t *testing.T) {
	services := ParseList(listMixed)
	if len(services) != 7 {
		t.Fatalf("expected 7 services and no header row, got %d", len(services))
	}

	l2tp := services[2]
	if l2tp.ID != "21E566BB-76AE-4D64-AD57-B2B14E830217" {
		t.Fatalf("unexpected id %q", l2tp.ID)
	}
	if l2tp.Name != "VPN CAT" {
		t.Fatalf("expected the quoted name, got %q", l2tp.Name)
	}
	if l2tp.Kind != "PPP:L2TP" {
		t.Fatalf("expected the bracketed kind, got %q", l2tp.Kind)
	}
	if !l2tp.Connected || l2tp.State != "Connected" {
		t.Fatalf("expected connected, got %v/%q", l2tp.Connected, l2tp.State)
	}
}

func TestSystemManagedSeparatesOSServicesFromAppServices(t *testing.T) {
	byName := map[string]Service{}
	for _, service := range ParseList(listMixed) {
		byName[service.Name] = service
	}

	for _, name := range []string{"VPN CAT", "Sweet Home", "VPN"} {
		if !byName[name].SystemManaged() {
			t.Fatalf("%q is configured in System Settings and macOS can drive it", name)
		}
		if id := byName[name].ProviderID(); id != "" {
			t.Fatalf("%q should have no owning app, got %q", name, id)
		}
	}

	for _, name := range []string{"Tailscale", "VPN 2", "Surfshark. WireGuard"} {
		if byName[name].SystemManaged() {
			t.Fatalf("%q belongs to a third-party app and must not be driven by scutil", name)
		}
	}
	if got := byName["Surfshark. WireGuard"].ProviderID(); got != "com.surfshark.vpnclient.macos.direct" {
		t.Fatalf("expected the owning bundle id, got %q", got)
	}
}

func TestFindByNameIsForgivingAboutCase(t *testing.T) {
	services := ParseList(listMixed)

	found, ok := FindByName(services, "vpn cat")
	if !ok || found.ID != "21E566BB-76AE-4D64-AD57-B2B14E830217" {
		t.Fatalf("expected a case-insensitive match, got ok=%v id=%q", ok, found.ID)
	}
	if _, ok := FindByName(services, "Nope"); ok {
		t.Fatal("an unknown name must not match")
	}
	if _, ok := FindByName(services, "  "); ok {
		t.Fatal("a blank name must not match")
	}
}

func TestSystemManagedNamesAreOfferedForErrorMessages(t *testing.T) {
	names := SystemManagedNames(ParseList(listMixed))
	want := []string{"VPN CAT", "Sweet Home", "VPN"}

	if len(names) != len(want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, names)
		}
	}
}

// statusL2TP is a connected L2TP service, the shape that publishes an address.
const statusL2TP = `Connected
Extended Status <dictionary> {
  DNSServers : <array> {
    0 : 1.0.0.1
    1 : 8.8.4.4
  }
  IPv4 : <dictionary> {
    Addresses : <array> {
      0 : 192.168.255.159
    }
    DestAddresses : <array> {
      0 : 192.168.255.1
    }
    InterfaceName : ppp0
    Router : 192.168.255.159
  }
  Status : 2
}
`

// statusNoAddress is a connected NetworkExtension service that publishes no
// IPv4 block at all — Tailscale reports this shape.
const statusNoAddress = `Connected
Extended Status <dictionary> {
  DNSServers : <array> {
    0 : 115.178.58.26
  }
  IsPrimaryInterface : 0
  Status : 2
  VPN : <dictionary> {
    ConnectTime : 8851
    RemoteAddress : 127.0.0.1
  }
}
`

const statusDisconnected = `Disconnected
Extended Status <dictionary> {
  NEStatus : 1
  SessionState : 1
  Status : 0
}
`

func TestParseStatusReadsTheLocalAddressNotThePeer(t *testing.T) {
	reading := ParseStatus(statusL2TP)

	if !reading.Connected {
		t.Fatal("expected connected")
	}
	if reading.AssignedIP != "192.168.255.159" {
		t.Fatalf("expected the local address, got %q (DestAddresses must not win)", reading.AssignedIP)
	}
	if reading.Interface != "ppp0" {
		t.Fatalf("expected ppp0, got %q", reading.Interface)
	}
}

func TestParseStatusInventsNoAddressWhenMacOSPublishesNone(t *testing.T) {
	reading := ParseStatus(statusNoAddress)

	if !reading.Connected {
		t.Fatal("expected connected")
	}
	if reading.AssignedIP != "" || reading.Interface != "" {
		t.Fatalf("expected no address to be invented, got %q/%q", reading.AssignedIP, reading.Interface)
	}
}

func TestParseStatusReadsDisconnected(t *testing.T) {
	reading := ParseStatus(statusDisconnected)

	if reading.Connected {
		t.Fatal("expected disconnected")
	}
	if reading.State != "Disconnected" {
		t.Fatalf("expected the state word, got %q", reading.State)
	}
}
