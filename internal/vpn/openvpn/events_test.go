package openvpn

import "testing"

const sampleLog = `
[Nov 17, 2026, 22:59:38] EVENT: CONNECTED minkdev@54.255.98.28:11194 (54.255.98.28) via /UDP on utun8/192.168.201.199/ gw=[192.168.201.1/] mtu=(default)
[Nov 17, 2026, 23:03:26] EVENT: DISCONNECTED
[Nov 17, 2026, 23:03:47] EVENT: DISCONNECTED
`

func TestReplayLogConnectedThenDisconnected(t *testing.T) {
	s, ok := replayLog(sampleLog)
	if ok {
		t.Fatalf("expected no active session after disconnect, got %+v", s)
	}
}

func TestReplayLogLeavesLastConnected(t *testing.T) {
	log := `
line
EVENT: CONNECTED alice@10.0.0.1:1194 (10.0.0.1) via /UDP on utun3/10.8.0.2/
EVENT: DISCONNECTED
EVENT: CONNECTED bob@10.0.0.2:1194 (10.0.0.2) via /UDP on utun4/10.8.0.3/
`
	s, ok := replayLog(log)
	if !ok {
		t.Fatal("expected active session")
	}
	if s.Username != "bob" || s.Host != "10.0.0.2" || s.AssignedIP != "10.8.0.3" {
		t.Fatalf("got %+v", s)
	}
}

func TestSessionMatchesTargetByHostAndUser(t *testing.T) {
	s := Session{Username: "minkdev", Host: "54.255.98.28", Resolved: "54.255.98.28"}
	if !s.matchesTarget("54.255.98.28", "minkdev") {
		t.Fatal("host+user should match")
	}
	if s.matchesTarget("54.255.98.28", "other") {
		t.Fatal("wrong user must not match")
	}
	if s.matchesTarget("203.0.113.1", "minkdev") {
		t.Fatal("wrong host must not match")
	}
}

func TestProfileByNameAcceptsFriendlyNameAndID(t *testing.T) {
	profiles := []Profile{
		{ID: "1794930963772", Name: "54.255.98.28 [pclo-aws-sg-ovpn-totp]", Host: "54.255.98.28", Username: "minkdev"},
		{ID: "2", Name: "10.0.0.9 [other]", Host: "10.0.0.9"},
	}
	for _, in := range []string{
		"54.255.98.28 [pclo-aws-sg-ovpn-totp]",
		"pclo-aws-sg-ovpn-totp",
		"1794930963772",
		"ovpn-totp",
	} {
		if p, ok := profileByName(profiles, in); !ok || p.ID != "1794930963772" {
			t.Fatalf("%q → %+v ok=%v", in, p, ok)
		}
	}
	if _, ok := profileByName(profiles, "."); ok {
		t.Fatal("an ambiguous partial must not match")
	}
	if _, ok := profileByName(profiles, ""); ok {
		t.Fatal("an empty value must not pick one of several profiles")
	}
}

func TestParseListProfilesJSON(t *testing.T) {
	raw := `[{"id":"1","name":"corp","host":"vpn.example.com","username":"ming","type":"user-locked"}]`
	got, err := parseListProfilesOutput(raw)
	if err != nil || len(got) != 1 || got[0].Name != "corp" {
		t.Fatalf("json: err=%v got=%+v", err, got)
	}
}

func TestParseListProfilesUnquotedKeys(t *testing.T) {
	raw := `[{
 id: abc,
 name: BMA_DC,
 host: 1.1.254.50,
 type: user-locked,
 username: ming,
 server-override: none
}]`
	got, err := parseListProfilesOutput(raw)
	if err != nil {
		t.Fatalf("unquoted keys: %v", err)
	}
	if len(got) != 1 || got[0].Name != "BMA_DC" || got[0].Host != "1.1.254.50" {
		t.Fatalf("got %+v", got)
	}
}