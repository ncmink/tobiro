package tailscale

import (
	"testing"

	"tobiro/internal/domain"
)

const switchListFixture = `[
  {
    "id": "0d3a",
    "nickname": "work",
    "tailnet": "corp.example.com",
    "account": "alice@corp.example.com",
    "selected": false
  },
  {
    "id": "1ab2",
    "nickname": "",
    "tailnet": "home.ts.net",
    "account": "bob@home.example.com",
    "selected": true
  }
]`

const minkSwitchListFixture = `[
  {
    "id": "0d3a",
    "nickname": "ttss-ponggun2527",
    "tailnet": "ponggun2527@gmail.com",
    "account": "nc_minkz@hotmail.com",
    "selected": true
  },
  {
    "id": "068a",
    "nickname": "ncmink",
    "tailnet": "nc_minkz@hotmail.com",
    "account": "nc_minkz@hotmail.com",
    "selected": false
  }
]`

func parseFixture(t *testing.T, raw string) []switchAccount {
	t.Helper()
	accounts, err := parseSwitchListJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return accounts
}

func TestParseSwitchListJSON(t *testing.T) {
	accounts := parseFixture(t, switchListFixture)
	if len(accounts) != 2 {
		t.Fatalf("len = %d, want 2", len(accounts))
	}
	if accounts[0].switchTarget() != "work" {
		t.Fatalf("first target = %q, want work", accounts[0].switchTarget())
	}
	if accounts[1].switchTarget() != "bob@home.example.com" {
		t.Fatalf("second target = %q", accounts[1].switchTarget())
	}
}

func TestAccountsToOptions(t *testing.T) {
	accounts := parseFixture(t, switchListFixture)
	opts := accountsToOptions(accounts)
	if len(opts) != 2 {
		t.Fatalf("options = %d, want 2", len(opts))
	}
	if opts[0].Name != "work" || opts[0].Connected {
		t.Fatalf("first option = %+v", opts[0])
	}
	if opts[1].Name != "home.ts.net" || !opts[1].Connected {
		t.Fatalf("second option = %+v", opts[1])
	}
	if opts[1].Detail != "home.ts.net · bob@home.example.com" {
		t.Fatalf("detail = %q", opts[1].Detail)
	}
}

func TestAccountMatches(t *testing.T) {
	if !accountMatches("Alice@Corp.com", "alice@corp.com") {
		t.Fatal("email match should be case-insensitive")
	}
	if !accountMatches("work", "work") {
		t.Fatal("nickname exact match")
	}
	if accountMatches("bob@home.example.com", "work") {
		t.Fatal("different accounts must not match")
	}
	if !accountMatches("anything", "") {
		t.Fatal("empty target matches legacy")
	}
}

func TestResolveProfileMinkFixture(t *testing.T) {
	accounts := parseFixture(t, minkSwitchListFixture)
	cases := []struct {
		vendor string
		wantID string
		want   resolveResult
	}{
		{"0d3a", "0d3a", resolveFound},
		{"ttss-ponggun2527", "0d3a", resolveFound},
		{"ponggun2527@gmail.com", "0d3a", resolveFound},
		{"068a", "068a", resolveFound},
		{"ncmink", "068a", resolveFound},
		{"nc_minkz@hotmail.com", "068a", resolveFound},
		{"missing", "", resolveNotFound},
	}
	for _, tc := range cases {
		got := resolveProfile(accounts, tc.vendor)
		if got.Result != tc.want {
			t.Fatalf("vendor %q: result = %v, want %v", tc.vendor, got.Result, tc.want)
		}
		if tc.want == resolveFound && got.Account.ID != tc.wantID {
			t.Fatalf("vendor %q: id = %q, want %q", tc.vendor, got.Account.ID, tc.wantID)
		}
	}
}

func TestResolveProfileAmbiguousSharedLogin(t *testing.T) {
	accounts := parseFixture(t, minkSwitchListFixture)
	// Login alone is ambiguous when two profiles share nc_minkz@hotmail.com — but our
	// tailnet rule matches nc_minkz@hotmail.com to profile 068a uniquely. Use a synthetic
	// fixture where two profiles share login without tailnet disambiguation via vendor login.
	raw := `[
	  {"id":"a1","nickname":"one","tailnet":"t1.example.com","account":"shared@example.com","selected":true},
	  {"id":"a2","nickname":"two","tailnet":"t2.example.com","account":"shared@example.com","selected":false}
	]`
	accounts = parseFixture(t, raw)
	got := resolveProfile(accounts, "shared@example.com")
	if got.Result != resolveAmbiguous {
		t.Fatalf("shared login: result = %v, want ambiguous", got.Result)
	}
}

func swFromFixture(t *testing.T, raw string) ActiveSwitch {
	t.Helper()
	return ActiveSwitch{Accounts: parseFixture(t, raw)}
}

func TestVPNStatusForProfileWrongAccount(t *testing.T) {
	reading := statusReading{
		BackendState: "Running",
		AssignedIP:   "100.64.1.1",
		Identity:     "alice@corp.example.com",
		Detail:       "Tailscale is up.",
	}
	sw := swFromFixture(t, switchListFixture)
	got := reading.vpnStatusForProfile("work", sw)
	if got.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected", got.State)
	}
	if got.Detail == "" || got.Detail == reading.Detail {
		t.Fatalf("detail = %q, want mismatch explanation", got.Detail)
	}
}

func TestVPNStatusForProfileMatchesNickname(t *testing.T) {
	raw := `[
	  {"id":"0d3a","nickname":"work","tailnet":"corp.example.com","account":"alice@corp.example.com","selected":true}
	]`
	reading := statusReading{
		BackendState: "Running",
		Identity:     "alice@corp.example.com",
		Detail:       "Tailscale is up.",
	}
	sw := swFromFixture(t, raw)
	got := reading.vpnStatusForProfile("work", sw)
	if got.State != domain.StateConnected {
		t.Fatalf("state = %s, want connected", got.State)
	}
}

func TestVPNStatusMinkSelectedProfile(t *testing.T) {
	reading := statusReading{
		BackendState: "Running",
		AssignedIP:   "100.114.130.24",
		Identity:     "nc_minkz@hotmail.com",
		Detail:       "Tailscale is up.",
	}
	sw := swFromFixture(t, minkSwitchListFixture)

	got := reading.vpnStatusForProfile("ttss-ponggun2527", sw)
	if got.State != domain.StateConnected {
		t.Fatalf("ttss-ponggun2527: state = %s, want connected", got.State)
	}
	gotOther := reading.vpnStatusForProfile("ncmink", sw)
	if gotOther.State != domain.StateDisconnected {
		t.Fatalf("ncmink: state = %s, want disconnected", gotOther.State)
	}
	gotTailnet := reading.vpnStatusForProfile("nc_minkz@hotmail.com", sw)
	if gotTailnet.State != domain.StateDisconnected {
		t.Fatalf("nc_minkz tailnet card while ponggun selected: state = %s, want disconnected", gotTailnet.State)
	}
}

func TestVPNStatusLegacyEmptyVendorProfile(t *testing.T) {
	reading := statusReading{BackendState: "Running", Detail: "Tailscale is up."}
	got := reading.vpnStatusForProfile("", ActiveSwitch{})
	if got.State != domain.StateConnected {
		t.Fatalf("state = %s, want connected for legacy", got.State)
	}
}

func TestVPNStatusEmptyVendorMultipleSwitchProfiles(t *testing.T) {
	reading := statusReading{
		BackendState: "Running",
		AssignedIP:   "100.114.130.24",
		Identity:     "nc_minkz@hotmail.com",
		Detail:       "Tailscale is up.",
	}
	sw := swFromFixture(t, minkSwitchListFixture)
	got := reading.vpnStatusForProfile("", sw)
	if got.State != domain.StateDisconnected {
		t.Fatalf("empty vendor with 2 profiles: state = %s, want disconnected", got.State)
	}
}

func TestVPNStatusEmptyVendorWithSeveralConnections(t *testing.T) {
	reading := statusReading{
		BackendState: "Running",
		AssignedIP:   "100.64.1.1",
		Identity:     "user@example.com",
		Detail:       "Tailscale is up.",
	}
	got := reading.vpnStatusForProfileOptions("", ActiveSwitch{Accounts: []switchAccount{{Selected: true}}}, false)
	if got.State != domain.StateDisconnected {
		t.Fatalf("state = %s, want disconnected when empty vendor is not allowed", got.State)
	}
}
